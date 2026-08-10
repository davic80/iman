package conectores

import (
	"strings"
	"testing"

	"github.com/davic80/iman/internal/titulos"
)

// Las fixtures son las secciones de películas de los tres sitios, capturadas
// desde el servidor el 10 de agosto de 2026.
//
// Lo que se prueba aquí no es solo que el parser saque filas: es que la página
// elegida sea **de películas y solo de películas**. Ese es el requisito de la
// portada de novedades, y se cumple eligiendo la URL, no filtrando después.

func TestNovedadesDeDivxTotalSonPeliculas(t *testing.T) {
	d := NuevoDivxTotal(nil)
	rs, err := d.parsearBusqueda(documento(t, "divxtotal-novedades.html"))
	if err != nil {
		t.Fatalf("parsearBusqueda: %v", err)
	}
	if len(rs) < 10 {
		t.Fatalf("salieron %d novedades, esperaba una página entera", len(rs))
	}
	for _, r := range rs {
		if strings.Contains(r.Ficha, "/series/") || strings.Contains(r.Ficha, "/programas/") {
			t.Errorf("se coló algo que no es película: %q", r.Ficha)
		}
		if r.Info.Idioma == titulos.Desconocido {
			t.Errorf("%q salió sin idioma", r.Titulo)
		}
	}
	// Es el único de los tres que publica la fecha de subida, y es lo que hace
	// que valga la pena leerla.
	if rs[0].Fecha.IsZero() {
		t.Errorf("%q salió sin fecha", rs[0].Titulo)
	}
}

func TestNovedadesDeEliteTorrentSonPeliculas(t *testing.T) {
	e := NuevoEliteTorrent(nil)
	rs, err := e.parsearBusqueda(documento(t, "elitetorrent-novedades.html"))
	if err != nil {
		t.Fatalf("parsearBusqueda: %v", err)
	}
	if len(rs) < 10 {
		t.Fatalf("salieron %d novedades, esperaba una página entera", len(rs))
	}
	for _, r := range rs {
		if !strings.Contains(r.Ficha, "/peliculas/") {
			t.Errorf("se coló algo que no es película: %q", r.Ficha)
		}
		if r.Titulo == "" {
			t.Error("una novedad salió sin título")
		}
	}
}

func TestNovedadesDeDonTorrentSonPeliculas(t *testing.T) {
	d := NuevoDonTorrent(nil)
	rs, err := d.parsearRejilla(documento(t, "dontorrent-novedades.html"))
	if err != nil {
		t.Fatalf("parsearRejilla: %v", err)
	}
	if len(rs) < 10 {
		t.Fatalf("salieron %d novedades, esperaba una página entera", len(rs))
	}
	for _, r := range rs {
		if !strings.Contains(r.Ficha, "/pelicula/") {
			t.Errorf("se coló algo que no es película: %q", r.Ficha)
		}
		if r.Titulo == "" {
			t.Error("una novedad salió sin título")
		}
		if r.Info.Idioma != titulos.Castellano {
			t.Errorf("%q salió en %v; aquí se supone castellano", r.Titulo, r.Info.Idioma)
		}
	}

	// La rejilla repite la misma película en varias secciones de la página.
	fichas := make(map[string]bool, len(rs))
	for _, r := range rs {
		if fichas[r.Ficha] {
			t.Errorf("%q sale dos veces", r.Ficha)
		}
		fichas[r.Ficha] = true
	}
}

// En DonTorrent el título no está escrito en ninguna parte: hay que sacarlo del
// nombre del fichero de la carátula, que es lo único que lo lleva junto con la
// calidad.
func TestElTituloDeDonTorrentSaleDeLaCaratula(t *testing.T) {
	d := NuevoDonTorrent(nil)
	rs, err := d.parsearRejilla(documento(t, "dontorrent-novedades.html"))
	if err != nil {
		t.Fatalf("parsearRejilla: %v", err)
	}

	var conCalidad int
	for _, r := range rs {
		if r.Info.Calidad != titulos.CalidadDesconocida {
			conCalidad++
		}
		// Si se hubiera cogido el nombre del fichero entero, los títulos
		// arrastrarían las etiquetas del sitio.
		if strings.ContainsAny(r.Titulo, "[]") || strings.Contains(r.Titulo, "DonTorrent") {
			t.Errorf("el título trae la etiquetería de la carátula: %q", r.Titulo)
		}
		if strings.Contains(r.Titulo, "-") {
			t.Errorf("el título conserva los guiones del fichero: %q", r.Titulo)
		}
	}
	if conCalidad == 0 {
		t.Error("ninguna novedad trajo calidad, y la carátula la lleva")
	}
}

// La ficha arregla el título que la rejilla dejó a medias. Es lo que permite
// que "Admisin imposible" acabe siendo "Admisión imposible" y se pueda fundir
// con la misma película de otro sitio.
func TestLaFichaDeDonTorrentCorrigeElTitulo(t *testing.T) {
	d := NuevoDonTorrent(nil)
	r := Resultado{
		Sitio:  d.Nombre(),
		Titulo: "Matrix Reloaded sin acentos",
		Ficha:  "https://tomadivx.net/pelicula/970/Matrix-Reloaded",
	}
	if err := d.parsearFicha(documento(t, "dontorrent-ficha.html"), &r); err != nil {
		t.Fatalf("parsearFicha: %v", err)
	}
	if r.Titulo != "Matrix Reloaded" {
		t.Errorf("Titulo = %q, quiero el de la ficha", r.Titulo)
	}
	// Y con el título nuevo hay que rehacer lo que se dedujo del viejo.
	if r.Info.Obra != titulos.Obra("Matrix Reloaded") {
		t.Errorf("Obra = %q, quiero la del título nuevo", r.Info.Obra)
	}
	// Sin pisar lo que la propia ficha declara aparte.
	if r.Info.Idioma != titulos.Castellano {
		t.Errorf("Idioma = %v", r.Info.Idioma)
	}
}

func TestDeLaCaratula(t *testing.T) {
	casos := []struct {
		src             string
		titulo, calidad string
	}{
		{
			"https://images.weserv.nl/?url=https://x.in/imagenes/peliculas/Carrera-de-bestias-[DVDRip]-[DonTorrent]-[Utag].jpg",
			"Carrera de bestias", "DVDRip",
		},
		{
			"https://x.in/imagenes/peliculas/La-ltima-casa-4K-[4K]-[DonTorrent]-[bKSB].jpg",
			"La ltima casa 4K", "4K",
		},
		{"https://x.in/imagenes/peliculas/Sin-etiquetas.jpg", "Sin etiquetas", ""},
	}
	for _, c := range casos {
		doc := documentoDeCadena(t, `<img src="`+c.src+`">`)
		titulo, calidad := deLaCaratula(doc.Find("img").First())
		if titulo != c.titulo || calidad != c.calidad {
			t.Errorf("deLaCaratula(%q) = %q/%q, quiero %q/%q",
				c.src, titulo, calidad, c.titulo, c.calidad)
		}
	}
}

// Sin carátula queda el slug de la URL, que es peor pero es algo.
func TestSinCaratulaSeUsaLaRuta(t *testing.T) {
	d := NuevoDonTorrent(nil)
	doc := documentoDeCadena(t, `<a href="/pelicula/30825/La-odisea"></a>`)
	rs, err := d.parsearRejilla(doc)
	if err != nil {
		t.Fatalf("parsearRejilla: %v", err)
	}
	if len(rs) != 1 || rs[0].Titulo != "La odisea" {
		t.Fatalf("salió %+v, quiero un resultado titulado \"La odisea\"", rs)
	}
}
