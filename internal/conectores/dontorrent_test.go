package conectores

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davic80/iman/internal/titulos"
)

// Las fixtures son capturas reales del 5 de agosto de 2026, tomadas desde el
// servidor de Alemania: la búsqueda de "matrix" y la ficha de Matrix Reloaded
// en tomadivx.net, más el parking de dontorrent.click con su interstitial.

// La búsqueda es un POST del formulario: la consulta va en "valor", y pedir
// /buscar/<consulta> da 404 desde que el sitio vive en dontorrent.moi.
func TestDonTorrentBuscaPorFormulario(t *testing.T) {
	var metodo, valor string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metodo, valor = r.Method, r.PostFormValue("valor")
		http.ServeFile(w, r, "testdata/dontorrent-moi-busqueda.html")
	}))
	defer srv.Close()

	d := NuevoDonTorrent(NuevoCliente(0))
	d.Mudar(srv.URL)
	rs, err := d.Buscar(context.Background(), " tadeo jones ")
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if metodo != http.MethodPost || valor != "tadeo jones" {
		t.Errorf("se pidió %s con valor %q", metodo, valor)
	}
	if len(rs) != 10 || rs[0].Titulo != "Las aventuras de Tadeo Jones [3D]." {
		t.Fatalf("%d resultados, el primero %+v", len(rs), rs)
	}
}

// La ficha de dontorrent.moi no tiene enlace: el .torrent sale tras una prueba
// de trabajo en el navegador. Se resuelve sin .torrent y sin error.
func TestDonTorrentFichaProtegida(t *testing.T) {
	srv := servidorConFixture(t, "dontorrent-moi-ficha.html")
	defer srv.Close()

	d := NuevoDonTorrent(NuevoCliente(0))
	d.Mudar(srv.URL)
	r := Resultado{Ficha: srv.URL + "/pelicula/25832/Tadeo-Jones-3"}
	if err := d.Resolver(context.Background(), &r); err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if r.Torrent != "" || r.Titulo != "Tadeo Jones 3" {
		t.Errorf("Torrent %q, Título %q", r.Torrent, r.Titulo)
	}
}

func TestDonTorrentParsearBusqueda(t *testing.T) {
	d := NuevoDonTorrent(nil)
	rs, err := d.parsearBusqueda(documento(t, "dontorrent-busqueda.html"))
	if err != nil {
		t.Fatalf("parsearBusqueda: %v", err)
	}

	// La página dice "Se han encontrado 13 resultados" pero pagina de diez.
	if len(rs) != 10 {
		t.Fatalf("salieron %d resultados, quiero 10", len(rs))
	}

	primero := rs[0]
	if primero.Titulo != "Matrix [4K]" {
		t.Errorf("Titulo = %q", primero.Titulo)
	}
	if primero.Ficha != DonTorrentBase+"/pelicula/23404/Matrix-4K" {
		t.Errorf("Ficha = %q", primero.Ficha)
	}
	if primero.Info.Calidad != titulos.Cal4K {
		t.Errorf("Calidad = %v, quiero 4K", primero.Info.Calidad)
	}
	// Este sitio no publica semillas: -1 es "no se sabe", que no es 0.
	if primero.SemillasConocidas() {
		t.Errorf("Semillas = %d, quiero desconocidas", primero.Semillas)
	}
}

// Es la razón de ser de idiomaPorDefecto: sin él todos los resultados de este
// sitio saldrían como Desconocido y quedarían escondidos tras el interruptor
// de dudosos, o sea que el conector no serviría de nada.
func TestSinMarcaDeIdiomaSeSuponeCastellano(t *testing.T) {
	d := NuevoDonTorrent(nil)
	rs, err := d.parsearBusqueda(documento(t, "dontorrent-busqueda.html"))
	if err != nil {
		t.Fatalf("parsearBusqueda: %v", err)
	}
	for _, r := range rs {
		if r.Info.Idioma != titulos.Castellano {
			t.Errorf("%q salió como %v, quiero castellano", r.Titulo, r.Info.Idioma)
		}
	}
}

// Y la otra mitad del trato: cuando el título sí trae marca, manda el título.
// Si esto se rompiera, Imán colaría latino como castellano, que es exactamente
// lo que el proyecto promete no hacer.
func TestElTituloManaSobreElIdiomaPorDefecto(t *testing.T) {
	casos := []struct {
		titulo string
		quiero titulos.Idioma
	}{
		{"Cartas a Julieta (Latino).", titulos.Latino},
		{"American Assassin [Audio Español-Latino]", titulos.Latino},
		{"Matrix Reloaded.", titulos.Castellano},
		{"Matrix [4K]", titulos.Castellano},
	}
	for _, c := range casos {
		got := idiomaPorDefecto(titulos.Analizar(c.titulo).Idioma)
		if got != c.quiero {
			t.Errorf("%q -> %v, quiero %v", c.titulo, got, c.quiero)
		}
	}
}

func TestDonTorrentParsearFicha(t *testing.T) {
	d := NuevoDonTorrent(nil)
	r := Resultado{Ficha: "https://tomadivx.net/pelicula/10058/Matrix-Reloaded"}
	if err := d.parsearFicha(documento(t, "dontorrent-ficha.html"), &r); err != nil {
		t.Fatalf("parsearFicha: %v", err)
	}

	// El enlace viene sin esquema ("//tomadivx.net/..."), así que si esto sale
	// bien es que se resolvió contra la base.
	quiero := "https://tomadivx.net/torrents/peliculas/matrix-reloaded-bdremux-1080-px.torrent"
	if r.Torrent != quiero {
		t.Errorf("Torrent = %q, quiero %q", r.Torrent, quiero)
	}
	// Este sitio no da magnets, solo el fichero.
	if r.Magnet != "" {
		t.Errorf("Magnet = %q, quiero vacío", r.Magnet)
	}
	// La ficha dice "16,4 GB", con la coma decimal española.
	gb := 16.4
	if quiero := int64(gb * float64(int64(1)<<30)); r.Tamaño != quiero {
		t.Errorf("Tamaño = %d, quiero %d", r.Tamaño, quiero)
	}
	if r.Info.Año != 2003 {
		t.Errorf("Año = %d, quiero 2003", r.Info.Año)
	}
}

// La huella se apoya en marcas de la plantilla, no de la marca comercial,
// porque este sitio vive en varios dominios con nombres distintos.
func TestLaHuellaDeDonTorrentReconoceSuHTML(t *testing.T) {
	srv := servidorConFixture(t, "dontorrent-busqueda.html")
	defer srv.Close()

	d := NuevoDonTorrent(NuevoCliente(0))
	sondeo, err := d.Sondear(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Sondear: %v", err)
	}
	if sondeo.Resultados < d.Huella().MinAciertos {
		t.Errorf("Resultados = %d, quiero al menos %d", sondeo.Resultados, d.Huella().MinAciertos)
	}
	for _, marca := range d.Huella().Contiene {
		if !strings.Contains(strings.ToLower(sondeo.HTML), marca) {
			t.Errorf("falta la marca %q", marca)
		}
	}
}

// El parking de dontorrent.click lleva la palabra "dontorrent" cinco veces y
// los espejos buenos ninguna. Este test existe para que a nadie le tiente
// meterla como marca positiva.
//
// Se prueban sus dos caras: el interstitial de FingerprintJS con el que recibe
// (que durante un tiempo creímos que era la defensa del sitio de verdad) y la
// página de anuncios que hay detrás.
func TestLaHuellaDeDonTorrentRechazaSuParking(t *testing.T) {
	for _, fixture := range []string{
		"dontorrent-parking-interstitial.html",
		"dontorrent-parking.html",
	} {
		t.Run(fixture, func(t *testing.T) {
			srv := servidorConFixture(t, fixture)
			defer srv.Close()

			d := NuevoDonTorrent(NuevoCliente(0))
			sondeo, err := d.Sondear(context.Background(), srv.URL)
			if err != nil {
				return // Que ni siquiera parsee también vale como rechazo.
			}
			if sondeo.Resultados >= d.Huella().MinAciertos {
				t.Errorf("el parking coló con %d resultados", sondeo.Resultados)
			}
		})
	}
}

// Sin Referer el sitio contesta 200 y una página sin resultados, así que un
// conector que no lo mande parece funcionar y no encuentra nada nunca.
func TestSeMandaElRefererQueElSitioExige(t *testing.T) {
	var recibido string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recibido = r.Header.Get("Referer")
		http.ServeFile(w, r, "testdata/dontorrent-busqueda.html")
	}))
	defer srv.Close()

	d := NuevoDonTorrent(NuevoCliente(0))
	d.Mudar(srv.URL)
	if _, err := d.Buscar(context.Background(), "matrix"); err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if quiero := srv.URL + "/"; recibido != quiero {
		t.Errorf("Referer = %q, quiero %q", recibido, quiero)
	}
}

func TestDonTorrentNoDescargaDeOtroSitio(t *testing.T) {
	d := NuevoDonTorrent(NuevoCliente(0))
	malas := []string{
		"https://otrositio.com/x.torrent",
		"file:///etc/passwd",
		"http://127.0.0.1:8080/admin",
		"https://tomadivx.net.malo.com/x.torrent",
	}
	for _, dir := range malas {
		if _, err := d.Descargar(context.Background(), dir); err == nil {
			t.Errorf("%q se aceptó y no debería", dir)
		}
	}
}

func TestSondearNoCambiaElDominioDeDonTorrent(t *testing.T) {
	srv := servidorConFixture(t, "dontorrent-busqueda.html")
	defer srv.Close()

	d := NuevoDonTorrent(NuevoCliente(0))
	antes := d.Base()
	if _, err := d.Sondear(context.Background(), srv.URL); err != nil {
		t.Fatalf("Sondear: %v", err)
	}
	if d.Base() != antes {
		t.Errorf("Sondear cambió la base de %q a %q", antes, d.Base())
	}
}

// servidorConFixture sirve un HTML guardado en cualquier ruta que se le pida.
func servidorConFixture(t *testing.T, nombre string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, "testdata/"+nombre)
	}))
}
