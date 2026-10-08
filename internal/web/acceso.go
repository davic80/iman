package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Acceso con Google.
//
// La instancia es de una sola persona: quien entra tiene que demostrar ante
// Google que es la cuenta de IMAN_PERMITIDO y nada más. No hay usuarios, ni
// base de datos, ni librería de OAuth: el flujo de código de autorización cabe
// en dos peticiones, y la sesión es una cookie firmada con HMAC que dice quién
// es y hasta cuándo vale.

const (
	cookieSesion   = "iman_sesion"
	cookieEstado   = "iman_estado"
	duracionSesion = 30 * 24 * time.Hour
)

// Las direcciones de Google van en variables para que los tests puedan poner
// un Google de mentira delante.
var (
	googleAutorizar = "https://accounts.google.com/o/oauth2/v2/auth"
	googleToken     = "https://oauth2.googleapis.com/token"
	googleUsuario   = "https://openidconnect.googleapis.com/v1/userinfo"
)

// accesoActivo dice si hay que pedir Google. Sin IMAN_GOOGLE_ID se arranca
// abierto, que es lo que hacen los tests y el desarrollo en local.
func (s *Servidor) accesoActivo() bool { return s.cfg.GoogleID != "" }

// rutasLibres no piden sesión: la sonda del contenedor, lo estático que pinta
// la pantalla de error y el propio flujo de entrada.
func rutaLibre(ruta string) bool {
	switch ruta {
	case "/vivo", "/entrar", "/oauth/google", "/salir":
		return true
	}
	return strings.HasPrefix(ruta, "/estaticos/")
}

// exigirSesion deja pasar solo a quien trae una cookie de sesión válida.
func (s *Servidor) exigirSesion(siguiente http.Handler) http.Handler {
	if !s.accesoActivo() {
		return siguiente
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rutaLibre(r.URL.Path) || s.sesion(r) != "" {
			siguiente.ServeHTTP(w, r)
			return
		}
		// Un navegador que pide una página va a Google; cualquier otra cosa
		// (un POST, un fetch) recibe un 401 seco en vez de una redirección
		// que no sabría seguir.
		if r.Method == http.MethodGet {
			http.Redirect(w, r, "/entrar", http.StatusFound)
			return
		}
		http.Error(w, "hace falta entrar", http.StatusUnauthorized)
	})
}

func (s *Servidor) redireccion() string {
	return strings.TrimRight(s.cfg.URLPublica, "/") + "/oauth/google"
}

// entrar manda a Google con un estado aleatorio que vuelve en la respuesta y
// se compara con la cookie: así nadie puede colarnos el código de otro.
func (s *Servidor) entrar(w http.ResponseWriter, r *http.Request) {
	estado := aleatorio()
	http.SetCookie(w, &http.Cookie{
		Name: cookieEstado, Value: estado, Path: "/oauth/google",
		MaxAge: 600, HttpOnly: true, Secure: s.cookiesSeguras(), SameSite: http.SameSiteLaxMode,
	})
	q := url.Values{
		"client_id":     {s.cfg.GoogleID},
		"redirect_uri":  {s.redireccion()},
		"response_type": {"code"},
		"scope":         {"openid email"},
		"state":         {estado},
		"login_hint":    {s.cfg.Permitido},
		"prompt":        {"select_account"},
	}
	http.Redirect(w, r, googleAutorizar+"?"+q.Encode(), http.StatusFound)
}

// vueltaGoogle canjea el código por un token, pregunta a Google quién es y,
// si es la cuenta permitida, abre sesión.
func (s *Servidor) vueltaGoogle(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieEstado)
	estado := r.URL.Query().Get("state")
	if err != nil || estado == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(estado)) != 1 {
		http.Error(w, "estado de OAuth no válido; vuelve a /entrar", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieEstado, Path: "/oauth/google", MaxAge: -1})

	codigo := r.URL.Query().Get("code")
	if codigo == "" {
		http.Error(w, "Google no ha devuelto código", http.StatusBadRequest)
		return
	}

	ctx, cancelar := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancelar()
	correo, err := s.preguntarGoogle(ctx, codigo)
	if err != nil {
		s.log.Warn("acceso con google", "error", err)
		http.Error(w, "no se pudo comprobar la cuenta con Google", http.StatusBadGateway)
		return
	}
	if !strings.EqualFold(correo, s.cfg.Permitido) {
		s.log.Warn("acceso denegado", "correo", correo)
		http.Error(w, "esta cuenta no tiene acceso", http.StatusForbidden)
		return
	}

	s.log.Info("acceso", "correo", correo)
	http.SetCookie(w, &http.Cookie{
		Name: cookieSesion, Value: s.firmar(correo, time.Now().Add(duracionSesion)), Path: "/",
		MaxAge: int(duracionSesion / time.Second), HttpOnly: true, Secure: s.cookiesSeguras(),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// preguntarGoogle hace el canje y la consulta de usuario. El token llega
// directo de Google por TLS, así que no hace falta verificar la firma del
// id_token: basta con preguntar a userinfo con él.
func (s *Servidor) preguntarGoogle(ctx context.Context, codigo string) (string, error) {
	form := url.Values{
		"code":          {codigo},
		"client_id":     {s.cfg.GoogleID},
		"client_secret": {s.cfg.GoogleSecreto},
		"redirect_uri":  {s.redireccion()},
		"grant_type":    {"authorization_code"},
	}
	pet, _ := http.NewRequestWithContext(ctx, http.MethodPost, googleToken, strings.NewReader(form.Encode()))
	pet.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := pedirJSON(pet, &token); err != nil {
		return "", fmt.Errorf("canjeando código: %w", err)
	}

	pet, _ = http.NewRequestWithContext(ctx, http.MethodGet, googleUsuario, nil)
	pet.Header.Set("Authorization", "Bearer "+token.AccessToken)
	var usuario struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := pedirJSON(pet, &usuario); err != nil {
		return "", fmt.Errorf("leyendo usuario: %w", err)
	}
	if !usuario.EmailVerified {
		return "", fmt.Errorf("correo %q sin verificar", usuario.Email)
	}
	return usuario.Email, nil
}

func pedirJSON(pet *http.Request, destino any) error {
	resp, err := http.DefaultClient.Do(pet)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s devolvió %d", pet.URL.Host, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(destino)
}

func (s *Servidor) salir(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieSesion, Path: "/", MaxAge: -1})
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "Sesión cerrada.")
}

// sesion devuelve el correo de la cookie si la firma es buena, no ha caducado
// y sigue siendo la cuenta permitida (por si se cambia IMAN_PERMITIDO).
func (s *Servidor) sesion(r *http.Request) string {
	c, err := r.Cookie(cookieSesion)
	if err != nil {
		return ""
	}
	datos, firma, ok := strings.Cut(c.Value, ".")
	if !ok || !hmac.Equal([]byte(firma), []byte(s.mac(datos))) {
		return ""
	}
	crudo, err := base64.RawURLEncoding.DecodeString(datos)
	if err != nil {
		return ""
	}
	correo, caduca, ok := strings.Cut(string(crudo), "|")
	n, err := strconv.ParseInt(caduca, 10, 64)
	if !ok || err != nil || time.Now().Unix() > n || !strings.EqualFold(correo, s.cfg.Permitido) {
		return ""
	}
	return correo
}

func (s *Servidor) firmar(correo string, caduca time.Time) string {
	datos := base64.RawURLEncoding.EncodeToString([]byte(correo + "|" + strconv.FormatInt(caduca.Unix(), 10)))
	return datos + "." + s.mac(datos)
}

func (s *Servidor) mac(datos string) string {
	m := hmac.New(sha256.New, s.claveSesion)
	m.Write([]byte(datos))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// cookiesSeguras: en producción la URL pública es https y las cookies solo
// viajan cifradas; en local por http no se marcan o el navegador las tiraría.
func (s *Servidor) cookiesSeguras() bool { return strings.HasPrefix(s.cfg.URLPublica, "https://") }

func aleatorio() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
