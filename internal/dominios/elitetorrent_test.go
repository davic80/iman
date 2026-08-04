package dominios

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davic80/iman/internal/conectores"
)

// Los tests de arriba usan un conector de mentira, así que prueban el resolutor
// pero no la huella de EliteTorrent. Y una huella mal escrita es un fallo
// silencioso de los caros: o rechaza el dominio bueno y el sitio desaparece de
// Imán, o acepta cualquier cosa y no sirve de nada.
//
// Estos dos sirven el HTML de verdad, el mismo que se capturó del sitio, desde
// un servidor local.

func htmlDeVerdad(t *testing.T) string {
	t.Helper()
	crudo, err := os.ReadFile(filepath.Join("..", "conectores", "testdata", "elitetorrent-busqueda.html"))
	if err != nil {
		t.Fatalf("leyendo el HTML capturado: %v", err)
	}
	return string(crudo)
}

func servir(t *testing.T, html string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func elite(t *testing.T) *conectores.EliteTorrent {
	t.Helper()
	// Sin pausa entre peticiones: es un servidor local y no hay a quién cuidar.
	return conectores.NuevoEliteTorrent(conectores.NuevoCliente(0))
}

func TestLaHuellaDeEliteTorrentReconoceSuHTML(t *testing.T) {
	e := elite(t)
	base := servir(t, htmlDeVerdad(t))

	r := Nuevo(&clienteFalso{}, mudo(), nil, e)
	sondeo, err := r.verificar(context.Background(), e, base)
	if err != nil {
		t.Fatalf("la huella rechaza el HTML auténtico del sitio: %v", err)
	}
	if sondeo.Resultados < e.Huella().MinAciertos {
		t.Errorf("resultados = %d, el mínimo de la huella es %d",
			sondeo.Resultados, e.Huella().MinAciertos)
	}
}

func TestLaHuellaDeEliteTorrentRechazaUnParking(t *testing.T) {
	e := elite(t)
	base := servir(t, `<html><head><title>elitetorrent.wf</title></head>
		<body><h1>elitetorrent.wf</h1>
		<p>Buy this domain</p>
		<div class="miniboxs-ficha"></div>
		<script src="/sk-park.php"></script></body></html>`)

	r := Nuevo(&clienteFalso{}, mudo(), nil, e)
	if _, err := r.verificar(context.Background(), e, base); err == nil {
		t.Fatal("la huella acepta un parking que se hace pasar por el sitio")
	}
}

// Un sitio entero que no es este: responde bien, pero no es EliteTorrent.
func TestLaHuellaDeEliteTorrentRechazaOtroSitio(t *testing.T) {
	e := elite(t)
	base := servir(t, `<html><title>Otro sitio de torrents</title>
		<ul class="resultados"><li>Matrix</li></ul></html>`)

	r := Nuevo(&clienteFalso{}, mudo(), nil, e)
	if _, err := r.verificar(context.Background(), e, base); err == nil {
		t.Fatal("la huella acepta un sitio que no es EliteTorrent")
	}
}

// Sondear no puede cambiar el dominio en uso: se llama para mirar candidatos, y
// mirar no es mudarse.
func TestSondearNoCambiaElDominioEnUso(t *testing.T) {
	e := elite(t)
	antes := e.Base()
	base := servir(t, htmlDeVerdad(t))

	if _, err := e.Sondear(context.Background(), base); err != nil {
		t.Fatalf("Sondear: %v", err)
	}
	if got := e.Base(); got != antes {
		t.Errorf("base = %q, sondear no debería haberla tocado (era %q)", got, antes)
	}
}

// El ciclo entero contra el HTML real: el dominio en uso está muerto, una
// semilla responde con el sitio auténtico, y se adopta sola.
func TestEliteTorrentSeMudaSolo(t *testing.T) {
	e := elite(t)
	vivo := servir(t, htmlDeVerdad(t))
	e.Mudar("http://127.0.0.1:1") // Un puerto donde no hay nadie

	dir := t.TempDir()
	estado, err := CargarEstado(filepath.Join(dir, "estado.json"))
	if err != nil {
		t.Fatalf("CargarEstado: %v", err)
	}

	r := Nuevo(&clienteFalso{}, mudo(), estado, e)
	// El conector no conoce el servidor de pruebas, así que se le pasa como si
	// fuera lo que sabe el estado guardado.
	if err := r.Revisar1(contextoCorto(t), sondaSemilla{e, vivo}); err != nil {
		t.Fatalf("Revisar1: %v", err)
	}
	if got := e.Base(); got != vivo {
		t.Errorf("base = %q, quiero %q", got, vivo)
	}
	// Y que quede apuntado para el siguiente arranque.
	if g, ok := estado.Dominio("EliteTorrent"); !ok || g != vivo {
		t.Errorf("estado = %q (%v), quiero %q", g, ok, vivo)
	}
}

// sondaSemilla envuelve un conector para añadirle una semilla que solo existe
// en el test: la URL del servidor local.
type sondaSemilla struct {
	conectores.Mudable
	extra string
}

func (s sondaSemilla) Semillas() []string {
	return append([]string{s.extra}, s.Mudable.Semillas()...)
}

func contextoCorto(t *testing.T) context.Context {
	t.Helper()
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancelar)
	return ctx
}
