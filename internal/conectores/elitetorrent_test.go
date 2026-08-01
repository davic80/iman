package conectores

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/davic80/iman/internal/titulos"
)

// Las fixtures son capturas reales de elitetorrent.wf del 1 de agosto de 2026:
// la búsqueda de "matrix" y la ficha del primer resultado. Los tests no tocan
// la red, así que el CI no depende de que el sitio esté vivo.
func documento(t *testing.T, nombre string) *goquery.Document {
	t.Helper()
	f, err := os.Open("testdata/" + nombre)
	if err != nil {
		t.Fatalf("abriendo fixture: %v", err)
	}
	defer f.Close()

	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("parseando fixture: %v", err)
	}
	return doc
}

func TestEliteTorrentURLBusqueda(t *testing.T) {
	e := NuevoEliteTorrent(nil)
	quiero := "https://www.elitetorrent.wf/?s=el+padrino"
	if got := e.URLBusqueda("el padrino"); got != quiero {
		t.Errorf("URLBusqueda = %q, quiero %q", got, quiero)
	}
}

func TestEliteTorrentParsearBusqueda(t *testing.T) {
	e := NuevoEliteTorrent(nil)
	rs, err := e.parsearBusqueda(documento(t, "elitetorrent-busqueda.html"))
	if err != nil {
		t.Fatalf("parsearBusqueda: %v", err)
	}

	// La propia página dice "Total 14 torrents" en su cabecera.
	if len(rs) != 14 {
		t.Fatalf("salieron %d resultados, quiero 14", len(rs))
	}

	primero := rs[0]
	if primero.Titulo != "The Matrix Resurrections" {
		t.Errorf("Titulo = %q", primero.Titulo)
	}
	if primero.Ficha != "https://www.elitetorrent.wf/peliculas/the-matrix-resurrections-3/" {
		t.Errorf("Ficha = %q", primero.Ficha)
	}
	if primero.Info.Idioma != titulos.Castellano {
		t.Errorf("Idioma = %v, quiero castellano", primero.Info.Idioma)
	}
	if primero.Info.Calidad != titulos.CalDVDRip {
		t.Errorf("Calidad = %v, quiero DVDRip", primero.Info.Calidad)
	}
	if quiero := gigas(2.21); primero.Tamaño != quiero {
		t.Errorf("Tamaño = %d, quiero %d", primero.Tamaño, quiero)
	}
	if primero.Sitio != "EliteTorrent" {
		t.Errorf("Sitio = %q", primero.Sitio)
	}

	// En la búsqueda no hay magnet: hay que ir a la ficha.
	if primero.Magnet != "" {
		t.Errorf("la búsqueda no debería traer magnet, trajo %q", primero.Magnet)
	}

	// Todos los resultados tienen que ser utilizables.
	for i, r := range rs {
		if r.Titulo == "" || r.Ficha == "" {
			t.Errorf("resultado %d incompleto: %+v", i, r)
		}
		if r.Tamaño <= 0 {
			t.Errorf("resultado %d sin tamaño: %q", i, r.Titulo)
		}
	}
}

// Este sitio etiqueta el idioma con una bandera, y es la única fuente de verdad
// que hay. Es justo lo que separa a Imán de buscar "castellano" en un sitio
// internacional, así que se comprueba que las tres etiquetas se leen bien.
func TestEliteTorrentDistingueLosTresIdiomas(t *testing.T) {
	e := NuevoEliteTorrent(nil)
	rs, err := e.parsearBusqueda(documento(t, "elitetorrent-busqueda.html"))
	if err != nil {
		t.Fatal(err)
	}

	// Los tres primeros resultados son la misma película con las tres
	// etiquetas distintas: castellano, latino y VOSE.
	quiero := []titulos.Idioma{titulos.Castellano, titulos.Latino, titulos.VOSE}
	for i, q := range quiero {
		if rs[i].Info.Idioma != q {
			t.Errorf("resultado %d: idioma %v, quiero %v", i, rs[i].Info.Idioma, q)
		}
	}

	var castellanos int
	for _, r := range rs {
		if r.Info.Idioma.Veredicto() == titulos.Acepta {
			castellanos++
		}
		if r.Info.Idioma == titulos.Desconocido {
			t.Errorf("resultado sin idioma: %q", r.Titulo)
		}
	}
	if castellanos == 0 {
		t.Error("ningún resultado en castellano; el filtro se comería la búsqueda entera")
	}
}

// Cuando la etiqueta de calidad viene vacía, el título a veces la lleva dentro:
// "Matrix (HDRip)". Perderla obligaría a enseñar "calidad desconocida" teniendo
// el dato delante.
func TestEliteTorrentCalidadDesdeElTitulo(t *testing.T) {
	e := NuevoEliteTorrent(nil)
	rs, err := e.parsearBusqueda(documento(t, "elitetorrent-busqueda.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if r.Titulo == "Matrix (HDRip)" {
			if r.Info.Calidad != titulos.CalHDRip {
				t.Errorf("Calidad de %q = %v, quiero HDRip", r.Titulo, r.Info.Calidad)
			}
			return
		}
	}
	t.Skip("la fixture ya no trae el resultado 'Matrix (HDRip)'")
}

func TestEliteTorrentParsearFicha(t *testing.T) {
	e := NuevoEliteTorrent(nil)
	r := Resultado{Ficha: "https://www.elitetorrent.wf/peliculas/the-matrix-resurrections-3/"}

	if err := e.parsearFicha(documento(t, "elitetorrent-ficha.html"), &r); err != nil {
		t.Fatalf("parsearFicha: %v", err)
	}

	const hash = "b457beaeb17a343999c335b523b705da6e9277ef"
	if !strings.HasPrefix(r.Magnet, "magnet:?xt=urn:btih:"+hash) {
		t.Errorf("Magnet = %q", r.Magnet)
	}
	if r.Torrent != "https://www.elitetorrent.wf/wp-content/uploads/2022/01/Matrix-Resurrections-2021.avi.torrent" {
		t.Errorf("Torrent = %q", r.Torrent)
	}
	if quiero := time.Date(2021, 12, 16, 0, 0, 0, 0, time.UTC); !r.Fecha.Equal(quiero) {
		t.Errorf("Fecha = %v, quiero %v", r.Fecha, quiero)
	}
	if r.Info.Idioma != titulos.Castellano {
		t.Errorf("Idioma = %v, quiero castellano", r.Info.Idioma)
	}
	if r.Info.Calidad != titulos.CalDVDRip {
		t.Errorf("Calidad = %v, quiero DVDRip", r.Info.Calidad)
	}
	if quiero := gigas(2.21); r.Tamaño != quiero {
		t.Errorf("Tamaño = %d, quiero %d", r.Tamaño, quiero)
	}
}

// gigas hace la misma cuenta que ParsearTamaño, con la misma aritmética de
// coma flotante. Escribir el número redondo a mano fallaría por un byte.
func gigas(n float64) int64 { return int64(n * float64(1<<30)) }

// Una ficha sin enlaces es un fallo del que hay que enterarse: significa que el
// sitio ha cambiado el HTML y el conector se ha quedado obsoleto.
func TestEliteTorrentFichaSinEnlacesDaError(t *testing.T) {
	e := NuevoEliteTorrent(nil)
	doc := documento(t, "elitetorrent-busqueda.html") // No es una ficha
	r := Resultado{Ficha: "https://www.elitetorrent.wf/lo-que-sea/"}

	if err := e.parsearFicha(doc, &r); err == nil {
		t.Error("quiero error al parsear algo que no es una ficha, no salió ninguno")
	}
}

func TestEliteTorrentCumpleLasInterfaces(t *testing.T) {
	var _ Conector = (*EliteTorrent)(nil)
	var _ Resolutor = (*EliteTorrent)(nil)
}
