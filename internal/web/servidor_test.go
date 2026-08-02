package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
)

const (
	fichaPrueba  = "https://sitio.test/peliculas/matrix/"
	magnetPrueba = "magnet:?xt=urn:btih:b457beaeb17a343999c335b523b705da6e9277ef&dn=matrix"
)

// sitioFalso es un conector de mentira que sirve de todo sin tocar la red.
type sitioFalso struct {
	resultados []conectores.Resultado
	err        error
	sinMagnet  bool
}

func (s *sitioFalso) Nombre() string  { return "Falso" }
func (s *sitioFalso) Dominio() string { return "sitio.test" }

func (s *sitioFalso) Buscar(ctx context.Context, consulta string) ([]conectores.Resultado, error) {
	return s.resultados, s.err
}

func (s *sitioFalso) Resolver(ctx context.Context, r *conectores.Resultado) error {
	if r.Ficha != fichaPrueba {
		return errors.New("esa ficha no es mía")
	}
	if !s.sinMagnet {
		r.Magnet = magnetPrueba
	}
	r.Torrent = "https://sitio.test/f/matrix.torrent"
	return nil
}

func (s *sitioFalso) Descargar(ctx context.Context, torrent string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("d8:announce...e")), nil
}

func resultado(titulo string, idioma titulos.Idioma) conectores.Resultado {
	return conectores.Resultado{
		Sitio:    "Falso",
		Titulo:   titulo,
		Ficha:    fichaPrueba,
		Tamaño:   2 * 1 << 30,
		Semillas: -1,
		Clientes: -1,
		Info: titulos.Info{
			Obra: titulo, Año: 1999, Idioma: idioma, Calidad: titulos.Cal1080p,
		},
	}
}

func mudo() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func servidorPrueba(t *testing.T, cs ...conectores.Conector) http.Handler {
	t.Helper()
	cfg := Config{Addr: ":0", Version: "prueba", TiempoBusqueda: 2 * time.Second}
	s, err := Nuevo(cfg, mudo(), buscador.Nuevo(mudo(), cfg.TiempoBusqueda, cs...))
	if err != nil {
		t.Fatalf("Nuevo: %v", err)
	}
	return s.Handler()
}

func pedir(t *testing.T, h http.Handler, ruta string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ruta, nil))
	return rec
}

func TestRutas(t *testing.T) {
	h := servidorPrueba(t)

	casos := []struct {
		ruta   string
		codigo int
		buscar string
	}{
		{"/", http.StatusOK, "Torrents en castellano"},
		{"/buscar?q=matrix", http.StatusOK, "matrix"},
		{"/salud", http.StatusOK, "Conectores"},
		{"/vivo", http.StatusOK, "ok"},
		{"/estaticos/estilo.css", http.StatusOK, "--iman"},
		{"/estaticos/iman.js", http.StatusOK, "clipboard"},
		{"/no-existe", http.StatusNotFound, ""},
	}

	for _, c := range casos {
		t.Run(c.ruta, func(t *testing.T) {
			rec := pedir(t, h, c.ruta)
			if rec.Code != c.codigo {
				t.Fatalf("codigo = %d, se esperaba %d", rec.Code, c.codigo)
			}
			if c.buscar != "" && !strings.Contains(rec.Body.String(), c.buscar) {
				t.Errorf("la respuesta no contiene %q", c.buscar)
			}
		})
	}
}

// La instancia es privada: si esta cabecera desaparece, acabamos indexados.
func TestNoIndex(t *testing.T) {
	h := servidorPrueba(t)
	for _, ruta := range []string{"/", "/salud"} {
		cuerpo := pedir(t, h, ruta).Body.String()
		if !strings.Contains(cuerpo, `name="robots"`) ||
			!strings.Contains(cuerpo, "noindex") {
			t.Errorf("%s no lleva meta robots noindex", ruta)
		}
	}
}

func TestAvisoSoloSinConectores(t *testing.T) {
	const aviso = "No hay ningún sitio conectado"

	if cuerpo := pedir(t, servidorPrueba(t), "/").Body.String(); !strings.Contains(cuerpo, aviso) {
		t.Error("sin conectores: falta el aviso")
	}
	con := pedir(t, servidorPrueba(t, &sitioFalso{}), "/").Body.String()
	if strings.Contains(con, aviso) {
		t.Error("con conectores: el aviso deberia haber desaparecido")
	}
}

func TestSaludPintaConectores(t *testing.T) {
	h := servidorPrueba(t, &sitioFalso{})
	cuerpo := pedir(t, h, "/salud").Body.String()

	// El dominio importa: cuando un sitio deja de devolver nada, lo primero
	// que hay que saber es si se ha mudado.
	for _, quiero := range []string{"Falso", "sitio.test", "pastilla-regular"} {
		if !strings.Contains(cuerpo, quiero) {
			t.Errorf("/salud no contiene %q", quiero)
		}
	}
}

func TestSaludReflejaLoQuePasa(t *testing.T) {
	h := servidorPrueba(t, &sitioFalso{
		resultados: []conectores.Resultado{resultado("Matrix", titulos.Castellano)},
	})

	// Antes de buscar nada no se sabe si el sitio está vivo.
	if cuerpo := pedir(t, h, "/salud").Body.String(); !strings.Contains(cuerpo, "sin datos") {
		t.Error("/salud debería decir 'sin datos' antes de la primera búsqueda")
	}

	pedir(t, h, "/buscar?q=matrix")

	cuerpo := pedir(t, h, "/salud").Body.String()
	if !strings.Contains(cuerpo, "pastilla-bien") {
		t.Error("/salud debería dar el sitio por vivo tras una búsqueda buena")
	}
}

func TestClaseSegunEstado(t *testing.T) {
	casos := map[string]string{
		buscador.Vivo:      "bien",
		buscador.Degradado: "regular",
		buscador.SinDatos:  "regular", // No se sabe, que no es lo mismo que bien
		buscador.Caido:     "mal",
		"cualquiera":       "mal", // lo desconocido se pinta como problema
	}
	for estado, clase := range casos {
		if c := (EstadoConector{Estado: estado}).Clase(); c != clase {
			t.Errorf("Clase(%q) = %q, se esperaba %q", estado, c, clase)
		}
	}
}

func TestBusquedaPintaResultados(t *testing.T) {
	h := servidorPrueba(t, &sitioFalso{resultados: []conectores.Resultado{
		resultado("The Matrix", titulos.Castellano),
	}})

	cuerpo := pedir(t, h, "/buscar?q=matrix").Body.String()
	for _, quiero := range []string{"The Matrix", "castellano", "1080p", "2.0 GB", "Copiar magnet"} {
		if !strings.Contains(cuerpo, quiero) {
			t.Errorf("la página de resultados no contiene %q", quiero)
		}
	}
}

// Lo que hace útil a Imán: que lo que no está en castellano no salga, y que se
// diga cuánto se ha quitado para que una lista vacía se entienda.
func TestBusquedaFiltraYLoDice(t *testing.T) {
	h := servidorPrueba(t, &sitioFalso{resultados: []conectores.Resultado{
		resultado("The Matrix latino", titulos.Latino),
		resultado("The Matrix vose", titulos.VOSE),
	}})

	cuerpo := pedir(t, h, "/buscar?q=matrix").Body.String()
	if strings.Contains(cuerpo, "The Matrix latino") || strings.Contains(cuerpo, "The Matrix vose") {
		t.Error("se han colado resultados que no son castellano")
	}
	if !strings.Contains(cuerpo, "Nada en castellano") {
		t.Error("falta explicar que no hay nada en castellano")
	}
	if !strings.Contains(cuerpo, "2 resultado") {
		t.Error("falta decir cuántos se descartaron")
	}
}

func TestBusquedaAvisaDeLosSitiosCaidos(t *testing.T) {
	h := servidorPrueba(t, &sitioFalso{err: errors.New("503")})

	cuerpo := pedir(t, h, "/buscar?q=matrix").Body.String()
	if !strings.Contains(cuerpo, "No contestaron") || !strings.Contains(cuerpo, "incompletos") {
		t.Error("con un sitio caído hay que decir que los resultados están incompletos")
	}
}

func TestSinConsultaNoSeBusca(t *testing.T) {
	h := servidorPrueba(t, &sitioFalso{resultados: []conectores.Resultado{
		resultado("The Matrix", titulos.Castellano),
	}})
	if cuerpo := pedir(t, h, "/buscar?q=").Body.String(); strings.Contains(cuerpo, "The Matrix") {
		t.Error("sin consulta no debería buscarse nada")
	}
}

func TestMagnetRedirige(t *testing.T) {
	h := servidorPrueba(t, &sitioFalso{})

	rec := pedir(t, h, "/magnet?sitio=Falso&ficha="+fichaPrueba)
	if rec.Code != http.StatusFound {
		t.Fatalf("codigo = %d, quiero %d", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != magnetPrueba {
		t.Errorf("Location = %q, quiero %q", got, magnetPrueba)
	}
}

// El botón de copiar necesita el magnet en texto: no puede seguir un redirect
// a magnet: porque el navegador se lo daría al cliente de torrents.
func TestMagnetEnTexto(t *testing.T) {
	h := servidorPrueba(t, &sitioFalso{})

	rec := pedir(t, h, "/magnet?formato=texto&sitio=Falso&ficha="+fichaPrueba)
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if got := rec.Body.String(); got != magnetPrueba {
		t.Errorf("cuerpo = %q, quiero el magnet", got)
	}
}

func TestMagnetErrores(t *testing.T) {
	casos := []struct {
		nombre string
		sitio  conectores.Conector
		ruta   string
		codigo int
	}{
		{"sin parámetros", &sitioFalso{}, "/magnet", http.StatusBadRequest},
		{"sin ficha", &sitioFalso{}, "/magnet?sitio=Falso", http.StatusBadRequest},
		// Una ficha que no es del sitio no puede acabar en una petición: si no,
		// Imán sería un proxy abierto para quien tenga la contraseña.
		{"ficha ajena", &sitioFalso{},
			"/magnet?sitio=Falso&ficha=http://169.254.169.254/", http.StatusBadGateway},
		{"sitio inventado", &sitioFalso{},
			"/magnet?sitio=Otro&ficha=" + fichaPrueba, http.StatusBadGateway},
		{"ficha sin magnet", &sitioFalso{sinMagnet: true},
			"/magnet?sitio=Falso&ficha=" + fichaPrueba, http.StatusNotFound},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			rec := pedir(t, servidorPrueba(t, c.sitio), c.ruta)
			if rec.Code != c.codigo {
				t.Errorf("codigo = %d, quiero %d", rec.Code, c.codigo)
			}
		})
	}
}

func TestTorrentSeSirveComoDescarga(t *testing.T) {
	h := servidorPrueba(t, &sitioFalso{})

	rec := pedir(t, h, "/torrent?sitio=Falso&ficha="+fichaPrueba)
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/x-bittorrent" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "matrix.torrent") {
		t.Errorf("Content-Disposition = %q", got)
	}
	if rec.Body.Len() == 0 {
		t.Error("el .torrent llegó vacío")
	}
}
