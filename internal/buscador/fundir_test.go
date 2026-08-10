package buscador

import (
	"testing"

	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
)

// pub arma un resultado como lo devolvería un conector: la obra ya deducida del
// título, y sin semillas, que es lo normal en los sitios españoles.
func pub(sitio, titulo string, cal titulos.Calidad, tamaño int64) conectores.Resultado {
	return conectores.Resultado{
		Sitio:    sitio,
		Titulo:   titulo,
		Ficha:    "https://" + sitio + "/" + titulo,
		Torrent:  "https://" + sitio + "/" + titulo + ".torrent",
		Tamaño:   tamaño,
		Semillas: -1,
		Clientes: -1,
		Info: titulos.Info{
			Obra:    titulos.Obra(titulo),
			Idioma:  titulos.Castellano,
			Calidad: cal,
		},
	}
}

func filas(t *testing.T, rs ...conectores.Resultado) []Resultado {
	t.Helper()
	return fundir(rs)
}

// El caso que motiva todo esto: la misma película en dos sitios, uno con el
// peso y el otro sin él, y el título escrito de forma distinta.
func TestFundeLaMismaPeliDeDosSitios(t *testing.T) {
	got := filas(t,
		pub("EliteTorrent", "The Matrix Resurrections", titulos.CalDVDRip, 2_200_000_000),
		pub("DonTorrent", "Matrix Resurrections", titulos.CalDVDRip, 0),
	)
	if len(got) != 1 {
		t.Fatalf("%d filas, quiero 1: %v", len(got), sitiosDe(got))
	}
	if q := []string{"EliteTorrent", "DonTorrent"}; !mismos(got[0].Sitios(), q) {
		t.Errorf("Sitios() = %v, quiero %v", got[0].Sitios(), q)
	}
}

// Lo que un sitio publica por separado se queda por separado. DivxTotal tiene
// la misma película en /peliculas y en /peliculas-hd, y son cosas distintas
// aunque las dos se llamen igual y ninguna diga su calidad.
func TestNoFundeDosEntradasDelMismoSitio(t *testing.T) {
	a := pub("DivxTotal", "Matrix Resurrections", titulos.CalidadDesconocida, 0)
	b := pub("DivxTotal", "Matrix Resurrections", titulos.CalidadDesconocida, 0)
	b.Ficha = "https://divxtotal.foo/peliculas-hd/matrix-resurrections/"

	if got := filas(t, a, b); len(got) != 2 {
		t.Errorf("%d filas, quiero 2: el sitio las publica aparte", len(got))
	}
}

func TestNoFundeCalidadesDistintas(t *testing.T) {
	got := filas(t,
		pub("Uno", "Matrix", titulos.Cal1080p, 0),
		pub("Dos", "Matrix", titulos.CalDVDRip, 0),
	)
	if len(got) != 2 {
		t.Errorf("%d filas, quiero 2: un DVDRip no es un 1080p", len(got))
	}
}

// Un peso desconocido no contradice a nadie, pero dos pesos que no cuadran sí:
// es el mismo título y la misma calidad, pero no el mismo fichero.
func TestElTamañoDecideCuandoSeSabe(t *testing.T) {
	casos := []struct {
		nombre string
		a, b   int64
		filas  int
	}{
		{"iguales", 2_000_000_000, 2_000_000_000, 1},
		{"redondeados distinto", 2_000_000_000, 2_050_000_000, 1},
		{"uno no lo dice", 2_000_000_000, 0, 1},
		{"ninguno lo dice", 0, 0, 1},
		{"no cuadran", 2_000_000_000, 8_000_000_000, 2},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := filas(t,
				pub("Uno", "Matrix", titulos.Cal1080p, c.a),
				pub("Dos", "Matrix", titulos.Cal1080p, c.b),
			)
			if len(got) != c.filas {
				t.Errorf("%d filas, quiero %d", len(got), c.filas)
			}
		})
	}
}

// Capítulos distintos de la misma serie no son el mismo torrent, por mucho que
// compartan obra.
func TestNoFundeCapitulosDistintos(t *testing.T) {
	a := pub("Uno", "Serie 1x01", titulos.Cal720p, 0)
	a.Info.Temporada, a.Info.Episodio = 1, 1
	b := pub("Dos", "Serie 1x02", titulos.Cal720p, 0)
	b.Info.Temporada, b.Info.Episodio = 1, 2
	b.Info.Obra = a.Info.Obra

	if got := filas(t, a, b); len(got) != 2 {
		t.Errorf("%d filas, quiero 2: son dos capítulos", len(got))
	}
}

// Sin obra no se puede afirmar nada: el año y la calidad por sí solos juntarían
// películas que no tienen nada que ver.
func TestSinObraNoSeFunde(t *testing.T) {
	a := pub("Uno", "", titulos.Cal1080p, 0)
	b := pub("Dos", "", titulos.Cal1080p, 0)
	b.Ficha = "https://dos/otra"

	if got := filas(t, a, b); len(got) != 2 {
		t.Errorf("%d filas, quiero 2: sin título no hay parecido que valga", len(got))
	}
}

// El infohash manda por encima de todo: si dos cosas son el mismo fichero, da
// igual cómo se llamen o de dónde vengan.
func TestElInfoHashFundeAunqueNoSeParezcan(t *testing.T) {
	const magnet = "magnet:?xt=urn:btih:b457beaeb17a343999c335b523b705da6e9277ef&dn=x"

	a := pub("Uno", "Matrix", titulos.Cal1080p, 2_000_000_000)
	a.Magnet = magnet
	b := pub("Dos", "Otra cosa que se llama distinto", titulos.CalDVDRip, 700_000_000)
	b.Magnet = magnet

	got := filas(t, a, b)
	if len(got) != 1 {
		t.Fatalf("%d filas, quiero 1: es el mismo fichero", len(got))
	}
}

// De los sitios que tienen lo mismo, delante va el que deja al usuario más
// cerca del contenido.
func TestMandaElQueDaElMagnet(t *testing.T) {
	conMagnet := pub("ConMagnet", "Matrix", titulos.Cal1080p, 0)
	conMagnet.Magnet = "magnet:?xt=urn:btih:b457beaeb17a343999c335b523b705da6e9277ef"
	soloFicha := pub("SoloFicha", "Matrix", titulos.Cal1080p, 0)
	soloFicha.Torrent = ""

	// En los dos órdenes, para que no gane por llegar antes.
	for _, rs := range [][]conectores.Resultado{
		{soloFicha, conMagnet},
		{conMagnet, soloFicha},
	} {
		got := filas(t, rs...)
		if len(got) != 1 {
			t.Fatalf("%d filas, quiero 1", len(got))
		}
		if got[0].Sitio != "ConMagnet" {
			t.Errorf("manda %q, quiero ConMagnet", got[0].Sitio)
		}
	}
}

// Tres sitios con lo mismo son una fila con tres, no dos filas.
func TestTresSitiosUnaFila(t *testing.T) {
	got := filas(t,
		pub("Uno", "Matrix", titulos.Cal1080p, 0),
		pub("Dos", "Matrix", titulos.Cal1080p, 0),
		pub("Tres", "Matrix", titulos.Cal1080p, 0),
	)
	if len(got) != 1 {
		t.Fatalf("%d filas, quiero 1", len(got))
	}
	if n := len(got[0].Sitios()); n != 3 {
		t.Errorf("%d sitios en la fila, quiero 3", n)
	}
}

// Cuando un sitio publica dos versiones y otro solo una, la que sobra no se
// pierde ni se mete a la fuerza: abre su propia fila.
func TestElQueNoCabeAbreFilaNueva(t *testing.T) {
	a := pub("DivxTotal", "Matrix", titulos.Cal1080p, 0)
	b := pub("DivxTotal", "Matrix", titulos.Cal1080p, 0)
	b.Ficha = "https://divxtotal.foo/peliculas-hd/matrix/"
	c := pub("DonTorrent", "Matrix", titulos.Cal1080p, 0)

	got := filas(t, a, b, c)
	if len(got) != 2 {
		t.Fatalf("%d filas, quiero 2: %v", len(got), sitiosDe(got))
	}
	// DonTorrent entra en la primera que lo admita y la otra se queda sola.
	if n := len(got[0].Sitios()); n != 2 {
		t.Errorf("la primera fila tiene %d sitios, quiero 2", n)
	}
	if n := len(got[1].Sitios()); n != 1 {
		t.Errorf("la segunda fila tiene %d sitios, quiero 1", n)
	}
}

// Al fundir no se copian datos de una publicación a otra: si un sitio no dice
// el peso, la fila no se inventa el del vecino.
func TestFundirNoSeInventaDatos(t *testing.T) {
	sinPeso := pub("DonTorrent", "Matrix", titulos.CalDVDRip, 0)
	sinPeso.Torrent = "" // Para que mande el otro
	conPeso := pub("EliteTorrent", "Matrix", titulos.CalDVDRip, 2_200_000_000)

	got := filas(t, sinPeso, conPeso)
	if len(got) != 1 {
		t.Fatalf("%d filas, quiero 1", len(got))
	}
	for _, o := range got[0].Repetidos {
		if o.Sitio == "DonTorrent" && o.Tamaño != 0 {
			t.Errorf("a DonTorrent le apareció un peso de %d", o.Tamaño)
		}
	}
}

func TestSinArticulo(t *testing.T) {
	casos := map[string]string{
		"the matrix resurrections": "matrix resurrections",
		"el padrino":               "padrino",
		"la leyenda del samurai":   "leyenda del samurai",
		"matrix":                   "matrix",
		"lassie":                   "lassie", // Empieza por "la" pero no es artículo
		"":                         "",
	}
	for entra, quiero := range casos {
		if got := sinArticulo(entra); got != quiero {
			t.Errorf("sinArticulo(%q) = %q, quiero %q", entra, got, quiero)
		}
	}
}

func sitiosDe(fs []Resultado) [][]string {
	out := make([][]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Sitios())
	}
	return out
}

func mismos(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
