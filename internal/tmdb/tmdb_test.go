package tmdb

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/davic80/iman/internal/titulos"
)

// Los ficheros de testdata son respuestas de verdad de TMDB, capturadas el 13
// de agosto de 2026 y recortadas a los tres primeros resultados. Los tests no
// salen a la red: quien comprueba que TMDB sigue contestando así es
// `tmdb_vivo_test.go`, que solo corre con -tags vivo y con clave.
//
// Cuatro de estas capturas son casos que aparecieron en la portada de verdad:
// dos películas homónimas, un título que el clasificador corta por la mitad,
// una coletilla entre paréntesis y una película que TMDB no tiene.

func silencio() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// sirve monta un TMDB de mentira que contesta siempre el mismo fichero y
// apunta cada petición que recibe.
func sirve(t *testing.T, fichero string) (*Cliente, *espia) {
	t.Helper()

	cuerpo, err := os.ReadFile(filepath.Join("testdata", fichero))
	if err != nil {
		t.Fatalf("no se pudo leer la captura: %v", err)
	}

	e := &espia{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.apuntar(r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(cuerpo)
	}))
	t.Cleanup(srv.Close)

	return clienteContra(srv.URL, "token-de-prueba"), e
}

func clienteContra(url, token string) *Cliente {
	return Nuevo(token, silencio()).Contra(url, url)
}

// espia guarda lo que le han pedido al TMDB de mentira.
type espia struct {
	mu       sync.Mutex
	veces    atomic.Int32
	rutas    []string
	consulta string
	auth     string
}

func (e *espia) apuntar(r *http.Request) {
	e.veces.Add(1)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rutas = append(e.rutas, r.URL.Path)
	e.consulta = r.URL.Query().Get("query")
	e.auth = r.Header.Get("Authorization")
	if k := r.URL.Query().Get("api_key"); k != "" {
		e.auth = "api_key=" + k
	}
}

func (e *espia) ruta() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.rutas) == 0 {
		return ""
	}
	return e.rutas[0]
}

func TestEncuentraLaPelicula(t *testing.T) {
	c, e := sirve(t, "matrix.json")

	f, hay := c.Buscar(context.Background(), "Matrix 1999 1080p Castellano", titulos.Analizar("Matrix 1999 1080p Castellano"))
	if !hay {
		t.Fatal("no encontró Matrix")
	}
	if f.ID != 603 {
		t.Errorf("ID = %d, quiero 603", f.ID)
	}
	if f.Titulo != "Matrix" {
		t.Errorf("Titulo = %q, quiero el castellano", f.Titulo)
	}
	if f.Año != 1999 {
		t.Errorf("Año = %d, quiero 1999", f.Año)
	}
	if f.Cartel == "" {
		t.Error("se quedó sin cartel")
	}
	if e.ruta() != "/search/movie" {
		t.Errorf("preguntó en %q, quiero /search/movie", e.ruta())
	}
}

// Los sitios españoles escriben "Matrix" donde TMDB tiene "The Matrix", y al
// revés. El artículo no puede costar una carátula.
func TestElArticuloNoEstorba(t *testing.T) {
	c, _ := sirve(t, "matrix.json")

	if _, hay := c.Buscar(context.Background(), "The Matrix 1999", titulos.Analizar("The Matrix 1999")); !hay {
		t.Error("no reconoció The Matrix como Matrix")
	}
}

// Dos películas con el mismo título son dos películas. El año es lo único que
// las distingue, y equivocarse aquí es poner el cartel del remake.
func TestElAñoDecideEntreHomonimas(t *testing.T) {
	c, _ := sirve(t, "dune.json")

	f, hay := c.Buscar(context.Background(), "Dune 1984 DVDRip", titulos.Analizar("Dune 1984 DVDRip"))
	if !hay {
		t.Fatal("no encontró Dune")
	}
	if f.ID != 841 {
		t.Errorf("ID = %d, quiero la del 84 (841) y no la del 21", f.ID)
	}
}

// Sin año no se puede elegir, así que se coge la que TMDB pone primera, que es
// la más conocida.
func TestSinAñoSeQuedaConLaMasConocida(t *testing.T) {
	c, _ := sirve(t, "dune.json")

	f, hay := c.Buscar(context.Background(), "Dune BluRay", titulos.Analizar("Dune BluRay"))
	if !hay || f.ID != 438631 {
		t.Errorf("ficha = %+v, quiero la primera que devuelve TMDB", f)
	}
}

// Preferir quedarse sin carátula: TMDB contesta algo a casi todo, y lo que
// contesta muchas veces es otra película con palabras parecidas. Aquí devuelve
// El padrino, El padrino II y otro El padrino de 2004; ninguno es el tercero.
func TestNoSeQuedaConLoQueNoEs(t *testing.T) {
	c, _ := sirve(t, "padrino.json")

	if f, hay := c.Buscar(context.Background(), "El padrino III 1990", titulos.Analizar("El padrino III 1990")); hay {
		t.Errorf("aceptó %q como El padrino III", f.Titulo)
	}
}

// Y cuando TMDB no tiene nada, no tiene nada: la captura de "Amo del universo"
// —una novedad real de un sitio— vuelve sin un solo resultado.
func TestCuandoTMDBNoSabeNada(t *testing.T) {
	c, _ := sirve(t, "amodeluniverso.json")

	if _, hay := c.Buscar(context.Background(), "Amo del universo", titulos.Analizar("Amo del universo")); hay {
		t.Error("se inventó una ficha con la lista vacía")
	}
}

// El clasificador corta el título por las marcas de release, y "capítulo" es
// una de ellas: "Una Milla: Capítulo Uno" se queda en "una milla", que es justo
// como TMDB no la llama. Por eso se compara también con el título entero.
func TestElTituloCortadoNoCuestaLaCaratula(t *testing.T) {
	c, _ := sirve(t, "unamilla.json")

	f, hay := c.Buscar(context.Background(), "Una Milla: Capítulo Uno", titulos.Analizar("Una Milla: Capítulo Uno"))
	if !hay {
		t.Fatal("no la reconoció")
	}
	if f.Titulo != "Una Milla: Capítulo Uno" {
		t.Errorf("Titulo = %q, y el capítulo dos no es el uno", f.Titulo)
	}
}

// TMDB desempata títulos repetidos con una coletilla entre paréntesis
// ("Incontrolable (I Swear)") que el sitio no escribe.
func TestElParentesisDeTMDBNoEstorba(t *testing.T) {
	c, _ := sirve(t, "incontrolable.json")

	f, hay := c.Buscar(context.Background(), "Incontrolable", titulos.Analizar("Incontrolable"))
	if !hay {
		t.Fatal("no reconoció Incontrolable")
	}
	if f.Titulo != "Incontrolable (I Swear)" {
		t.Errorf("Titulo = %q", f.Titulo)
	}
}

func TestLasSeriesSeBuscanEnSuSitio(t *testing.T) {
	c, e := sirve(t, "serie.json")

	f, hay := c.Buscar(context.Background(), "Breaking Bad S01E02 720p", titulos.Analizar("Breaking Bad S01E02 720p"))
	if !hay {
		t.Fatal("no encontró la serie")
	}
	if f.Titulo != "Breaking Bad" {
		t.Errorf("Titulo = %q", f.Titulo)
	}
	if e.ruta() != "/search/tv" {
		t.Errorf("preguntó en %q, quiero /search/tv", e.ruta())
	}
}

// El año de un capítulo es el del capítulo, no el del estreno de la serie: si
// vetara como en las películas, ninguna serie tendría carátula.
func TestElAñoDeUnCapituloNoTumbaLaSerie(t *testing.T) {
	c, _ := sirve(t, "serie.json")

	if _, hay := c.Buscar(context.Background(), "Breaking Bad 5x14 2012", titulos.Analizar("Breaking Bad 5x14 2012")); !hay {
		t.Error("el año del capítulo se cargó la ficha de la serie")
	}
}

// En la portada hay doce filas de la misma película en calidades distintas. Sin
// esto serían doce preguntas idénticas a TMDB por cada visita.
func TestNoPreguntaDosVecesLoMismo(t *testing.T) {
	c, e := sirve(t, "matrix.json")

	var grupo sync.WaitGroup
	for i := 0; i < 10; i++ {
		grupo.Add(1)
		go func() {
			defer grupo.Done()
			if _, hay := c.Buscar(context.Background(), "Matrix 1999 1080p", titulos.Analizar("Matrix 1999 1080p")); !hay {
				t.Error("una de las diez se quedó sin ficha")
			}
		}()
	}
	grupo.Wait()

	if n := e.veces.Load(); n != 1 {
		t.Errorf("%d peticiones a TMDB, quiero 1", n)
	}
}

// Un TMDB caído un momento no puede dejar sin carátula a esa película para
// siempre: eso es lo que pasaría si el fallo se guardara como "no está".
func TestUnFalloNoSeGuardaComoRespuesta(t *testing.T) {
	cuerpo, err := os.ReadFile(filepath.Join("testdata", "matrix.json"))
	if err != nil {
		t.Fatal(err)
	}

	var veces atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if veces.Add(1) == 1 {
			http.Error(w, "vuelve luego", http.StatusInternalServerError)
			return
		}
		w.Write(cuerpo)
	}))
	defer srv.Close()

	c := clienteContra(srv.URL, "token-de-prueba")
	info := titulos.Analizar("Matrix 1999 1080p")

	if _, hay := c.Buscar(context.Background(), "Matrix 1999 1080p", info); hay {
		t.Fatal("con TMDB caído no puede salir una ficha")
	}
	if _, hay := c.Buscar(context.Background(), "Matrix 1999 1080p", info); !hay {
		t.Error("no volvió a intentarlo después del fallo")
	}
}

// Sin clave configurada, Imán no habla con TMDB en absoluto. Ni una petición.
func TestSinClaveNoPregunta(t *testing.T) {
	var veces atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		veces.Add(1)
	}))
	defer srv.Close()

	c := clienteContra(srv.URL, "")
	if c.Activo() {
		t.Error("un cliente sin clave no está activo")
	}
	if _, hay := c.Buscar(context.Background(), "Matrix 1999", titulos.Analizar("Matrix 1999")); hay {
		t.Error("sin clave no puede haber ficha")
	}
	if n := veces.Load(); n != 0 {
		t.Errorf("%d peticiones sin clave, quiero 0", n)
	}
}

// Un cliente nil tiene que poder usarse: quien pinta la página no debería tener
// que preguntar si TMDB está configurado.
func TestElClienteNiloNoRevienta(t *testing.T) {
	var c *Cliente
	if c.Activo() {
		t.Error("nil no está activo")
	}
	if _, hay := c.Buscar(context.Background(), "Matrix 1999", titulos.Analizar("Matrix 1999")); hay {
		t.Error("nil no encuentra nada")
	}
}

// TMDB da dos credenciales en la misma página y son fáciles de confundir. Las
// dos valen, cada una por su camino.
func TestLasDosCredencialesValen(t *testing.T) {
	casos := []struct {
		nombre string
		token  string
		quiero string
	}{
		{"token nuevo", "eyJhbGciOiJIUzI1NiJ9.token.largo", "Bearer eyJhbGciOiJIUzI1NiJ9.token.largo"},
		{"clave de siempre", "0123456789abcdef0123456789abcdef", "api_key=0123456789abcdef0123456789abcdef"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			c, e := sirve(t, "matrix.json")
			c.token, c.v3 = caso.token, esClaveV3(caso.token)

			c.Buscar(context.Background(), "Matrix 1999", titulos.Analizar("Matrix 1999"))

			e.mu.Lock()
			defer e.mu.Unlock()
			if e.auth != caso.quiero {
				t.Errorf("mandó %q, quiero %q", e.auth, caso.quiero)
			}
		})
	}
}

// La ruta del cartel llega en la URL que pide el navegador: sin comprobarla,
// Imán se convierte en un proxy contra lo que le pongan.
func TestElCartelSoloSirveRutasDeTMDB(t *testing.T) {
	var veces atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		veces.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("\xff\xd8\xff imagen"))
	}))
	defer srv.Close()

	c := clienteContra(srv.URL, "token-de-prueba")

	malas := []string{
		"http://malo.test/robo.jpg",
		"//malo.test/robo.jpg",
		"/../../etc/passwd",
		"/abc.jpg?x=1",
		"/corto.jpg",
		"",
	}
	for _, mala := range malas {
		if _, _, err := c.Cartel(context.Background(), mala); err == nil {
			t.Errorf("aceptó %q como carátula", mala)
		}
	}
	if n := veces.Load(); n != 0 {
		t.Errorf("salió a la red %d veces con rutas malas, quiero 0", n)
	}

	// Los nombres de TMDB son base64 con guiones y subrayados: si se dejaran
	// fuera, esas carátulas —que son muchas— no se verían nunca.
	buenas := []string{
		"/dXNAPwY7VrqMAo51EKhhCJfaGb5.jpg",
		"/9O1Iy9od7-GeQ_xHYuoM3PtQzLM.jpg",
		"/3xnWaLQjelJDDF7LT1WBo6f4BRe.png",
	}
	for _, buena := range buenas {
		cuerpo, tipo, err := c.Cartel(context.Background(), buena)
		if err != nil {
			t.Fatalf("no trajo %q: %v", buena, err)
		}
		cuerpo.Close()
		if !strings.HasPrefix(tipo, "image/") {
			t.Errorf("tipo = %q", tipo)
		}
	}
}

// Una respuesta que no es una imagen no se le pasa al navegador como si lo
// fuera: si TMDB contesta una página de error, aquí se corta.
func TestElCartelNoSirveLoQueNoEsImagen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html>error</html>")
	}))
	defer srv.Close()

	c := clienteContra(srv.URL, "token-de-prueba")
	if _, _, err := c.Cartel(context.Background(), "/dXNAPwY7VrqMAo51EKhhCJfaGb5.jpg"); err == nil {
		t.Error("coló una página HTML como carátula")
	}
}
