package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func servidorPrueba(t *testing.T, estado FuenteEstado) http.Handler {
	t.Helper()
	cfg := Config{Addr: ":0", Version: "prueba", TiempoBusqueda: time.Second}
	s, err := Nuevo(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), estado)
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
	h := servidorPrueba(t, nil)

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
	h := servidorPrueba(t, nil)
	for _, ruta := range []string{"/", "/salud"} {
		cuerpo := pedir(t, h, ruta).Body.String()
		if !strings.Contains(cuerpo, `name="robots"`) ||
			!strings.Contains(cuerpo, "noindex") {
			t.Errorf("%s no lleva meta robots noindex", ruta)
		}
	}
}

// El aviso de "no hay conectores" solo debe salir mientras no los haya. Cuando
// llegue la fase 1 este test avisara de que hay que quitarlo.
func TestAvisoSoloSinConectores(t *testing.T) {
	const aviso = "Todavía no hay conectores"

	sin := pedir(t, servidorPrueba(t, nil), "/").Body.String()
	if !strings.Contains(sin, aviso) {
		t.Error("sin conectores: falta el aviso")
	}

	con := pedir(t, servidorPrueba(t, func() []EstadoConector {
		return []EstadoConector{{Nombre: "EliteTorrent", Estado: "vivo"}}
	}), "/").Body.String()
	if strings.Contains(con, aviso) {
		t.Error("con conectores: el aviso deberia haber desaparecido")
	}
}

func TestSaludPintaConectores(t *testing.T) {
	h := servidorPrueba(t, func() []EstadoConector {
		return []EstadoConector{{
			Nombre:  "EliteTorrent",
			Estado:  "vivo",
			Dominio: "elitetorrent.wf",
			Errores: 0,
		}}
	})

	cuerpo := pedir(t, h, "/salud").Body.String()
	for _, quiero := range []string{"EliteTorrent", "elitetorrent.wf", "pastilla-bien"} {
		if !strings.Contains(cuerpo, quiero) {
			t.Errorf("/salud no contiene %q", quiero)
		}
	}
}

func TestClaseSegunEstado(t *testing.T) {
	casos := map[string]string{
		"vivo":       "bien",
		"degradado":  "regular",
		"caido":      "mal",
		"cualquiera": "mal", // lo desconocido se pinta como problema
	}
	for estado, clase := range casos {
		if c := (EstadoConector{Estado: estado}).Clase(); c != clase {
			t.Errorf("Clase(%q) = %q, se esperaba %q", estado, c, clase)
		}
	}
}
