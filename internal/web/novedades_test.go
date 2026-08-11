package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/novedades"
	"github.com/davic80/iman/internal/titulos"
)

// sitioConNovedades es un sitio de mentira que además de buscar enseña lo último
// que ha subido.
type sitioConNovedades struct {
	*sitioFalso
	ultimas []conectores.Resultado
	rondas  int
}

func (s *sitioConNovedades) Novedades(context.Context) ([]conectores.Resultado, error) {
	s.rondas++
	return s.ultimas, nil
}

func novedad(sitio, titulo string) conectores.Resultado {
	info := titulos.Analizar(titulo)
	info.Idioma = titulos.Castellano
	return conectores.Resultado{
		Sitio:    sitio,
		Titulo:   titulo,
		Ficha:    "https://" + strings.ToLower(sitio) + ".test/pelicula/" + titulos.Normalizar(titulo),
		Tamaño:   2 * 1 << 30,
		Semillas: -1,
		Clientes: -1,
		Info:     info,
	}
}

// servidorConPortada monta un servidor con el rondín ya lleno, sin red de por
// medio y sin esperar a ninguna ronda de verdad.
func servidorConPortada(t *testing.T, rs ...conectores.Resultado) (http.Handler, *novedades.Rondin) {
	t.Helper()

	sitio := &sitioConNovedades{sitioFalso: &sitioFalso{}, ultimas: rs}
	cfg := Config{Addr: ":0", Version: "prueba", TiempoBusqueda: 2 * time.Second}
	s, err := Nuevo(cfg, mudo(), buscador.Nuevo(mudo(), cfg.TiempoBusqueda))
	if err != nil {
		t.Fatalf("Nuevo: %v", err)
	}

	rondin := novedades.Nuevo(mudo(), nil, sitio)
	rondin.Rondar(context.Background())
	s.ConNovedades(rondin)

	return s.Handler(), rondin
}

func TestLaPortadaEnseñaLoApuntado(t *testing.T) {
	h, _ := servidorConPortada(t,
		novedad("Falso", "El padrino 1972 DVDRip"),
		novedad("Falso", "Matrix 1999 1080p"),
	)

	rec := pedir(t, h, "/novedades")
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}
	cuerpo := rec.Body.String()
	for _, q := range []string{"El padrino", "Matrix", "2 películas"} {
		if !strings.Contains(cuerpo, q) {
			t.Errorf("la portada no contiene %q", q)
		}
	}
}

// Sin rondín enganchado la página existe igual: es lo que se ve en el primer
// arranque, antes de que acabe la primera ronda.
func TestLaPortadaVaciaLoDice(t *testing.T) {
	cuerpo := pedir(t, servidorPrueba(t), "/novedades").Body.String()
	if !strings.Contains(cuerpo, "No hay ningún sitio conectado") {
		t.Errorf("sin sitios: falta el aviso, salió %q", primeraLinea(cuerpo))
	}
}

func TestSePuedeCambiarElOrden(t *testing.T) {
	h, _ := servidorConPortada(t, novedad("Falso", "Matrix 1999 1080p"))

	porSitios := pedir(t, h, "/novedades?orden=sitios").Body.String()
	if !strings.Contains(porSitios, `pestaña--activa" href="/novedades?orden=sitios"`) {
		t.Error("la pestaña de sitios no queda marcada como activa")
	}

	// Y lo que llegue raro por la URL no puede tirar la página.
	rec := pedir(t, h, "/novedades?orden=loquesea")
	if rec.Code != http.StatusOK {
		t.Errorf("un orden inventado devolvió %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `pestaña--activa" href="/novedades?orden=fecha"`) {
		t.Error("un orden inventado debería caer en el de siempre")
	}
}

// El botón devuelve el control enseguida: recorrer los sitios lleva minutos y
// la petición del navegador no puede quedarse colgada todo ese rato.
func TestElBotonDeActualizarNoEsperaALaRonda(t *testing.T) {
	h, rondin := servidorConPortada(t, novedad("Falso", "Matrix 1999 1080p"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/novedades/refrescar",
		strings.NewReader("orden=sitios"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("codigo = %d, quiero un 303", rec.Code)
	}
	if destino := rec.Header().Get("Location"); destino != "/novedades?orden=sitios" {
		t.Errorf("Location = %q, quiero volver a la portada con su orden", destino)
	}

	// La ronda va por su cuenta; lo que importa aquí es que la respuesta ya
	// estaba escrita antes de que acabe.
	esperarRonda(t, rondin)
}

func esperarRonda(t *testing.T, r *novedades.Rondin) {
	t.Helper()
	limite := time.Now().Add(2 * time.Second)
	for r.Rondando() {
		if time.Now().After(limite) {
			t.Fatal("la ronda de fondo no terminó")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestHace(t *testing.T) {
	ahora := time.Now()
	casos := []struct {
		cuando time.Time
		quiero string
	}{
		{ahora, "ahora mismo"},
		{ahora.Add(-90 * time.Second), "hace 1 minuto"},
		{ahora.Add(-20 * time.Minute), "hace 20 minutos"},
		{ahora.Add(-70 * time.Minute), "hace 1 hora"},
		{ahora.Add(-5 * time.Hour), "hace 5 horas"},
		{ahora.Add(-30 * time.Hour), "hace 1 día"},
		{ahora.Add(-72 * time.Hour), "hace 3 días"},
	}
	for _, c := range casos {
		if got := hace(c.cuando); got != c.quiero {
			t.Errorf("hace(%v) = %q, quiero %q", c.cuando, got, c.quiero)
		}
	}
}

func primeraLinea(s string) string {
	if i := strings.Index(s, "\n"); i > 0 {
		return s[:i]
	}
	return s
}
