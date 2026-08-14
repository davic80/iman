package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/novedades"
	"github.com/davic80/iman/internal/tmdb"
)

// unaPelicula es lo que contesta el TMDB de mentira: la forma justa que Imán
// mira, no la respuesta entera, que trae veinte campos más.
const unaPelicula = `{"results":[{"id":603,"title":"Matrix","original_title":"The Matrix",
	"release_date":"1999-03-31","poster_path":"/dXNAPwY7VrqMAo51EKhhCJfaGb5.jpg",
	"overview":"Un programador descubre que el mundo no es lo que parece.",
	"genre_ids":[28,878],"vote_average":8.255,"vote_count":28438}]}`

// tmdbFalso monta un TMDB que contesta lo que se le diga y cuenta las visitas.
func tmdbFalso(t *testing.T, mano http.HandlerFunc) (*tmdb.Cliente, *atomic.Int32) {
	t.Helper()

	var veces atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		veces.Add(1)
		mano(w, r)
	}))
	t.Cleanup(srv.Close)

	return tmdb.Nuevo("token-de-prueba", mudo()).Contra(srv.URL, srv.URL), &veces
}

func sirveFicha(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/search/") {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, unaPelicula)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Write([]byte("\xff\xd8\xff carátula"))
}

// servidorConCarteles es como servidorConPortada pero con TMDB enganchado.
func servidorConCarteles(t *testing.T, c *tmdb.Cliente, rs ...conectores.Resultado) http.Handler {
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
	s.ConTMDB(c)

	return s.Handler()
}

func TestLaPortadaEnseñaLaCaratula(t *testing.T) {
	c, _ := tmdbFalso(t, sirveFicha)
	h := servidorConCarteles(t, c, novedad("Falso", "Matrix 1999 1080p"))

	cuerpo := pedir(t, h, "/novedades").Body.String()
	if !strings.Contains(cuerpo, `src="/cartel/w185/dXNAPwY7VrqMAo51EKhhCJfaGb5.jpg"`) {
		t.Errorf("no salió la carátula, salió %q", primeraLinea(cuerpo))
	}
	// La imagen se pide a Imán, nunca a image.tmdb.org: esta instancia es
	// privada y el navegador no tiene que ir contándolo por ahí.
	if strings.Contains(cuerpo, "image.tmdb.org") {
		t.Error("la página manda al navegador a TMDB")
	}
	// El crédito es la condición de TMDB para dejar usar la API.
	if !strings.Contains(cuerpo, "themoviedb.org") {
		t.Error("falta el crédito a TMDB en el pie")
	}
}

// La búsqueda pinta las carátulas igual que la portada: el mismo torrent no
// puede tener dos caras según por dónde se entre.
func TestLaBusquedaEnseñaLaCaratula(t *testing.T) {
	c, _ := tmdbFalso(t, sirveFicha)

	sitio := &sitioFalso{resultados: []conectores.Resultado{novedad("Falso", "Matrix 1999 1080p")}}
	cfg := Config{Addr: ":0", Version: "prueba", TiempoBusqueda: 2 * time.Second}
	s, err := Nuevo(cfg, mudo(), buscador.Nuevo(mudo(), cfg.TiempoBusqueda, sitio))
	if err != nil {
		t.Fatalf("Nuevo: %v", err)
	}
	s.ConTMDB(c)

	cuerpo := pedir(t, s.Handler(), "/buscar?q=matrix").Body.String()
	if !strings.Contains(cuerpo, `src="/cartel/w185/dXNAPwY7VrqMAo51EKhhCJfaGb5.jpg"`) {
		t.Errorf("no salió la carátula, salió %q", primeraLinea(cuerpo))
	}
}

// El título que se enseña sigue siendo el del sitio, con su calidad y sus
// marcas: es el que identifica a esta copia, no a la película.
func TestLaCaratulaNoCambiaElTitulo(t *testing.T) {
	c, _ := tmdbFalso(t, sirveFicha)
	h := servidorConCarteles(t, c, novedad("Falso", "Matrix 1999 1080p"))

	cuerpo := pedir(t, h, "/novedades").Body.String()
	if !strings.Contains(cuerpo, "Matrix 1999 1080p") {
		t.Error("se perdió el título del sitio")
	}
}

// TMDB es adorno. Que se caiga no puede dejar la portada sin películas.
func TestSiTMDBSeCaeLaPaginaSaleIgual(t *testing.T) {
	c, veces := tmdbFalso(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "estoy malo", http.StatusInternalServerError)
	})
	h := servidorConCarteles(t, c, novedad("Falso", "Matrix 1999 1080p"))

	rec := pedir(t, h, "/novedades")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Matrix") {
		t.Error("la película desapareció porque TMDB estaba caído")
	}
	if veces.Load() == 0 {
		t.Error("ni siquiera lo intentó")
	}
}

// Sin clave no se habla con TMDB, y además no se pinta el hueco de la carátula:
// una columna vacía en todas las filas no la quiere nadie.
func TestSinClaveNoHayHuecoDeCaratula(t *testing.T) {
	h, _ := servidorConPortada(t, novedad("Falso", "Matrix 1999 1080p"))

	if cuerpo := pedir(t, h, "/novedades").Body.String(); strings.Contains(cuerpo, "cartel") {
		t.Error("sin TMDB no debería haber ni hueco de carátula")
	}
}

func TestElCartelLoSirveIman(t *testing.T) {
	c, _ := tmdbFalso(t, sirveFicha)
	h := servidorConCarteles(t, c)

	rec := pedir(t, h, "/cartel/w185/dXNAPwY7VrqMAo51EKhhCJfaGb5.jpg")
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quiero 200", rec.Code)
	}
	if tipo := rec.Header().Get("Content-Type"); !strings.HasPrefix(tipo, "image/") {
		t.Errorf("Content-Type = %q", tipo)
	}
	if rec.Header().Get("Cache-Control") == "" {
		t.Error("sin Cache-Control el navegador la pide una y otra vez")
	}
}

// La ruta llega desde el navegador: si no se comprobara, /cartel sería un proxy
// contra cualquier cosa que alguien quisiera pedir en nombre del servidor.
func TestElCartelNoEsUnProxyAbierto(t *testing.T) {
	c, veces := tmdbFalso(t, sirveFicha)
	h := servidorConCarteles(t, c)

	for _, mala := range []string{"/cartel/w185/..%2f..%2fetc%2fpasswd", "/cartel/w185/robo.php", "/cartel/w185/x.jpg"} {
		if rec := pedir(t, h, mala); rec.Code == http.StatusOK {
			t.Errorf("%s devolvió 200", mala)
		}
	}
	if n := veces.Load(); n != 0 {
		t.Errorf("salió a la red %d veces, quiero 0", n)
	}
}

// Sin TMDB configurado la ruta existe igual, porque el mux es el mismo: lo que
// no puede es reventar.
func TestElCartelSinClaveContesta404(t *testing.T) {
	h, _ := servidorConPortada(t)

	if rec := pedir(t, h, "/cartel/w185/dXNAPwY7VrqMAo51EKhhCJfaGb5.jpg"); rec.Code != http.StatusNotFound {
		t.Errorf("código = %d, quiero 404", rec.Code)
	}
}

// El género y la nota salen en la fila. Es lo que TMDB sabe de la obra, y va
// junto a las etiquetas que dice el sitio.
func TestLaFilaEnseñaGeneroYPuntuacion(t *testing.T) {
	c, _ := tmdbFalso(t, sirveFicha)
	h := servidorConCarteles(t, c, novedad("Falso", "Matrix 1999 1080p"))

	cuerpo := pedir(t, h, "/novedades").Body.String()
	for _, quiero := range []string{"Acción · Ciencia ficción", "★ 8,3"} {
		if !strings.Contains(cuerpo, quiero) {
			t.Errorf("falta %q en la fila", quiero)
		}
	}
	// Con coma, que es como se escriben los decimales en castellano.
	if strings.Contains(cuerpo, "8.3") {
		t.Error("la nota salió con punto")
	}
}

// La carátula se puede ampliar: es un botón con lo que necesita el visor, y la
// imagen grande no se descarga hasta que se pincha.
func TestLaCaratulaSeAmplia(t *testing.T) {
	c, _ := tmdbFalso(t, sirveFicha)
	h := servidorConCarteles(t, c, novedad("Falso", "Matrix 1999 1080p"))

	cuerpo := pedir(t, h, "/novedades").Body.String()
	if !strings.Contains(cuerpo, `class="cartel-abrir"`) {
		t.Error("la carátula no es pinchable")
	}
	if !strings.Contains(cuerpo, `data-grande="/cartel/w500/dXNAPwY7VrqMAo51EKhhCJfaGb5.jpg"`) {
		t.Error("el botón no lleva la carátula grande")
	}
	// La grande solo aparece como dato del botón, nunca como src: si estuviera
	// en un <img> el navegador se bajaría setenta imágenes de golpe.
	if strings.Contains(cuerpo, `src="/cartel/w500/`) {
		t.Error("la lista se está descargando las carátulas grandes")
	}
	if !strings.Contains(cuerpo, `id="visor"`) {
		t.Error("falta el visor en la página")
	}
}

// Sin TMDB no hay visor que enseñar, así que tampoco se pinta.
func TestSinClaveNoHayVisor(t *testing.T) {
	h, _ := servidorConPortada(t, novedad("Falso", "Matrix 1999 1080p"))

	if cuerpo := pedir(t, h, "/novedades").Body.String(); strings.Contains(cuerpo, "visor") {
		t.Error("sin TMDB no debería haber visor")
	}
}

// El tamaño también llega desde el navegador, así que también hay que mirarlo:
// si no, /cartel/original sería una forma de hacer que este servidor se
// descargue las imágenes más gordas de TMDB una y otra vez.
func TestElCartelNoSirveCualquierTamaño(t *testing.T) {
	c, veces := tmdbFalso(t, sirveFicha)
	h := servidorConCarteles(t, c)

	fichero := "dXNAPwY7VrqMAo51EKhhCJfaGb5.jpg"
	for _, malo := range []string{"original", "w1280", "h632"} {
		if rec := pedir(t, h, "/cartel/"+malo+"/"+fichero); rec.Code == http.StatusOK {
			t.Errorf("/cartel/%s devolvió 200", malo)
		}
	}
	if n := veces.Load(); n != 0 {
		t.Errorf("salió a la red %d veces con tamaños malos, quiero 0", n)
	}

	if rec := pedir(t, h, "/cartel/w500/"+fichero); rec.Code != http.StatusOK {
		t.Errorf("el tamaño grande sí se sirve: código = %d", rec.Code)
	}
}
