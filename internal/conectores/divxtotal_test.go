package conectores

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/davic80/iman/internal/titulos"
)

// Las fixtures son capturas reales del 6 de agosto de 2026 tomadas desde el
// servidor: la búsqueda de "matrix", la ficha de Matrix Reloaded, una ficha de
// serie con sus 62 capítulos, y el parking en que se ha convertido divxtotal.tv.

func TestDivxTotalURLBusqueda(t *testing.T) {
	d := NuevoDivxTotal(nil)
	quiero := "https://divxtotal.foo/?s=el+padrino"
	if got := d.URLBusqueda("el padrino"); got != quiero {
		t.Errorf("URLBusqueda = %q, quiero %q", got, quiero)
	}
}

func TestDivxTotalParsearBusqueda(t *testing.T) {
	d := NuevoDivxTotal(nil)
	rs, err := d.parsearBusqueda(documento(t, "divxtotal-busqueda.html"))
	if err != nil {
		t.Fatalf("parsearBusqueda: %v", err)
	}

	// La página dice "23 torrents encontrados" pero pagina de quince.
	if len(rs) != 15 {
		t.Fatalf("salieron %d resultados, quiero 15", len(rs))
	}

	primero := rs[0]
	if primero.Titulo != "Matrix Resurrections" {
		t.Errorf("Titulo = %q", primero.Titulo)
	}
	if primero.Ficha != "https://divxtotal.foo/peliculas-hd/matrix-resurrections/" {
		t.Errorf("Ficha = %q", primero.Ficha)
	}
	// La fecha va en formato español, 18-01-2022.
	if q := time.Date(2022, 1, 18, 0, 0, 0, 0, time.UTC); !primero.Fecha.Equal(q) {
		t.Errorf("Fecha = %v, quiero %v", primero.Fecha, q)
	}
	// Este sitio no publica semillas: -1 es "no se sabe", que no es 0.
	if primero.SemillasConocidas() {
		t.Errorf("Semillas = %d, quiero desconocidas", primero.Semillas)
	}
}

// La columna de peso viene "N/A" en casi todos los resultados, y "N/A" no es
// un tamaño de cero bytes: es que no lo dicen.
func TestElPesoNoDisponibleNoSeInventa(t *testing.T) {
	d := NuevoDivxTotal(nil)
	rs, err := d.parsearBusqueda(documento(t, "divxtotal-busqueda.html"))
	if err != nil {
		t.Fatalf("parsearBusqueda: %v", err)
	}
	for _, r := range rs {
		if r.Tamaño < 0 {
			t.Errorf("%q salió con tamaño %d", r.Titulo, r.Tamaño)
		}
	}
}

func TestDivxTotalParsearFicha(t *testing.T) {
	d := NuevoDivxTotal(nil)
	r := Resultado{Ficha: "https://divxtotal.foo/peliculas/matrix-reloaded/"}
	if err := d.parsearFicha(documento(t, "divxtotal-ficha.html"), &r); err != nil {
		t.Fatalf("parsearFicha: %v", err)
	}

	quiero := "https://divxtotal.foo/wp-content/uploads/2016/10/Matrix-Reloaded-2003.avi.torrent"
	if r.Torrent != quiero {
		t.Errorf("Torrent = %q, quiero %q", r.Torrent, quiero)
	}
	// Este sitio no da magnets, solo el fichero.
	if r.Magnet != "" {
		t.Errorf("Magnet = %q, quiero vacío", r.Magnet)
	}
	// La ficha declara "Español", que es castellano.
	if r.Info.Idioma != titulos.Castellano {
		t.Errorf("Idioma = %v, quiero castellano", r.Info.Idioma)
	}
	if r.Info.Calidad != titulos.DetectarCalidad("DVDrip") {
		t.Errorf("Calidad = %v", r.Info.Calidad)
	}
}

// Es la razón de ser de todo el conector: el botón que se ve manda a un
// acortador de terceros, y ese enlace no vale. Si esto se rompiera, Imán
// mandaría a la gente a una cadena de anuncios en vez de darle el fichero.
func TestSeCogeElEnlacePropioYNoElDelAcortador(t *testing.T) {
	doc := documento(t, "divxtotal-ficha.html")

	// Que conste que el acortador está ahí, para que el test no pase por
	// casualidad si un día el sitio deja de ponerlo.
	html, err := doc.Html()
	if err != nil {
		t.Fatalf("Html: %v", err)
	}
	if !strings.Contains(html, "short-info.link") {
		t.Fatal("la fixture ya no trae el acortador: hay que revisar el conector")
	}

	enlace, err := enlaceTorrent(doc)
	if err != nil {
		t.Fatalf("enlaceTorrent: %v", err)
	}
	if strings.Contains(enlace, "short-info.link") {
		t.Errorf("se coló el acortador: %q", enlace)
	}
	if !strings.HasSuffix(enlace, ".torrent") {
		t.Errorf("el enlace no acaba en .torrent: %q", enlace)
	}
}

// Una ficha de serie trae una fila por capítulo. Se coge la primera, pero lo
// que importa es que el data-src elegido sea de verdad uno de ellos.
func TestUnaSerieDevuelveElEnlaceDeUnCapitulo(t *testing.T) {
	enlace, err := enlaceTorrent(documento(t, "divxtotal-serie.html"))
	if err != nil {
		t.Fatalf("enlaceTorrent: %v", err)
	}
	if !strings.HasPrefix(enlace, "https://divxtotal.foo/") {
		t.Errorf("el enlace no es del sitio: %q", enlace)
	}
	if !strings.HasSuffix(enlace, ".torrent") {
		t.Errorf("el enlace no acaba en .torrent: %q", enlace)
	}
}

// Un data-src que no sea base64, o que decodifique a otra cosa, tiene que dar
// error y no colarse: de ahí sale una URL que luego se pide.
func TestUnDataSrcRotoNoPasa(t *testing.T) {
	casos := map[string]string{
		"no es base64":  "esto-no-es-base64-!!!",
		"vacío":         "",
		"base64 vacío":  base64.StdEncoding.EncodeToString([]byte("   ")),
		"otro dominio":  base64.StdEncoding.EncodeToString([]byte("https://malo.com/x.torrent")),
		"no es una url": base64.StdEncoding.EncodeToString([]byte("file:///etc/passwd")),
	}
	d := NuevoDivxTotal(NuevoCliente(0))
	for nombre, codificado := range casos {
		html := `<a class="linktorrent" data-src="` + codificado + `" href="x">Descargar</a>`
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}

		enlace, err := enlaceTorrent(doc)
		if err != nil {
			continue // Rechazado al decodificar, que es lo que se quiere.
		}
		// Si decodificó, el filtro de dominio tiene que pararlo igual.
		if err := d.esMia(enlace); err == nil {
			t.Errorf("%s: %q se aceptó y no debería", nombre, enlace)
		}
	}
}

// La huella se apoya en lo que el sitio hace, no en cómo se llama.
func TestLaHuellaDeDivxTotalReconoceSuHTML(t *testing.T) {
	srv := servidorConFixture(t, "divxtotal-busqueda.html")
	defer srv.Close()

	d := NuevoDivxTotal(NuevoCliente(0))
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

// divxtotal.tv es hoy un parking, y sigue en la lista de semillas. Tiene que
// rebotar solo.
func TestLaHuellaDeDivxTotalRechazaSuParking(t *testing.T) {
	srv := servidorConFixture(t, "divxtotal-parking.html")
	defer srv.Close()

	d := NuevoDivxTotal(NuevoCliente(0))
	sondeo, err := d.Sondear(context.Background(), srv.URL)
	if err != nil {
		return // Que ni siquiera parsee también vale como rechazo.
	}
	if sondeo.Resultados >= d.Huella().MinAciertos {
		t.Errorf("el parking coló con %d resultados", sondeo.Resultados)
	}
}

func TestDivxTotalNoDescargaDeOtroSitio(t *testing.T) {
	d := NuevoDivxTotal(NuevoCliente(0))
	malas := []string{
		"https://otrositio.com/x.torrent",
		"https://short-info.link/s.php?i=abc",
		"file:///etc/passwd",
		"http://127.0.0.1:8080/admin",
		"https://divxtotal.foo.malo.com/x.torrent",
	}
	for _, dir := range malas {
		if _, err := d.Descargar(context.Background(), dir); err == nil {
			t.Errorf("%q se aceptó y no debería", dir)
		}
	}
}

func TestSondearNoCambiaElDominioDeDivxTotal(t *testing.T) {
	srv := servidorConFixture(t, "divxtotal-busqueda.html")
	defer srv.Close()

	d := NuevoDivxTotal(NuevoCliente(0))
	antes := d.Base()
	if _, err := d.Sondear(context.Background(), srv.URL); err != nil {
		t.Fatalf("Sondear: %v", err)
	}
	if d.Base() != antes {
		t.Errorf("Sondear cambió la base de %q a %q", antes, d.Base())
	}
}
