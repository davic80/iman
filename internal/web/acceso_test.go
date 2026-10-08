package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// googleFalso hace de Google: canjea cualquier código y dice que el usuario es
// el correo que se le pase.
func googleFalso(t *testing.T, correo string) {
	t.Helper()
	g := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
		case "/usuario":
			json.NewEncoder(w).Encode(map[string]any{"email": correo, "email_verified": true})
		}
	}))
	t.Cleanup(g.Close)
	viejos := [2]string{googleToken, googleUsuario}
	googleToken, googleUsuario = g.URL+"/token", g.URL+"/usuario"
	t.Cleanup(func() { googleToken, googleUsuario = viejos[0], viejos[1] })
}

func servidorConAcceso(t *testing.T) *Servidor {
	t.Helper()
	cfg := Config{Addr: ":0", Version: "prueba", TiempoBusqueda: time.Second,
		GoogleID: "id", GoogleSecreto: "secreto", URLPublica: "http://iman.test",
		Permitido: "david.cornejo@gmail.com", ClaveSesion: "clave"}
	s, err := Nuevo(cfg, mudo(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// entrarCon recorre el flujo entero y devuelve la respuesta de la vuelta.
func entrarCon(t *testing.T, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/entrar", nil))
	destino, _ := url.Parse(rec.Header().Get("Location"))
	estado := destino.Query().Get("state")
	if estado == "" {
		t.Fatalf("/entrar no manda a Google: %v", destino)
	}
	pet := httptest.NewRequest("GET", "/oauth/google?code=c&state="+estado, nil)
	for _, c := range rec.Result().Cookies() {
		pet.AddCookie(c)
	}
	vuelta := httptest.NewRecorder()
	h.ServeHTTP(vuelta, pet)
	return vuelta
}

func TestSinSesionRedirigeAEntrar(t *testing.T) {
	h := servidorConAcceso(t).Handler()
	rec := pedir(t, h, "/")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/entrar" {
		t.Fatalf("código %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := pedir(t, h, "/vivo"); rec.Code != http.StatusOK {
		t.Fatalf("/vivo tiene que ser libre, dio %d", rec.Code)
	}
}

func TestCuentaPermitidaEntra(t *testing.T) {
	googleFalso(t, "David.Cornejo@gmail.com")
	h := servidorConAcceso(t).Handler()
	vuelta := entrarCon(t, h)
	if vuelta.Code != http.StatusFound {
		t.Fatalf("vuelta dio %d: %s", vuelta.Code, vuelta.Body)
	}
	pet := httptest.NewRequest("GET", "/salud", nil)
	for _, c := range vuelta.Result().Cookies() {
		pet.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, pet)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Salir") {
		t.Fatalf("con sesión /salud dio %d", rec.Code)
	}
}

func TestOtraCuentaNoEntra(t *testing.T) {
	googleFalso(t, "otro@gmail.com")
	if vuelta := entrarCon(t, servidorConAcceso(t).Handler()); vuelta.Code != http.StatusForbidden {
		t.Fatalf("otra cuenta dio %d", vuelta.Code)
	}
}

func TestCookieManipuladaNoVale(t *testing.T) {
	s := servidorConAcceso(t)
	buena := s.firmar("david.cornejo@gmail.com", time.Now().Add(time.Hour))
	caducada := s.firmar("david.cornejo@gmail.com", time.Now().Add(-time.Hour))
	for nombre, valor := range map[string]string{
		"buena": buena, "caducada": caducada, "firma rota": buena + "x",
	} {
		pet := httptest.NewRequest("GET", "/", nil)
		pet.AddCookie(&http.Cookie{Name: cookieSesion, Value: valor})
		if got, want := s.sesion(pet) != "", nombre == "buena"; got != want {
			t.Errorf("%s: sesión %v, se esperaba %v", nombre, got, want)
		}
	}
}
