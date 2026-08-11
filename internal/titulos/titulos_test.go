package titulos

import "testing"

// Los casos de idioma salen de títulos reales de EliteTorrent, DonTorrent y
// The Pirate Bay. Los tres primeros son literalmente las tres etiquetas que
// usa EliteTorrent, que son las que fijan el filtro.
func TestDetectarIdioma(t *testing.T) {
	casos := []struct {
		texto  string
		quiero Idioma
	}{
		{"Pelicula en Español Castellano", Castellano},
		{"Pelicula en Español Latino", Latino},
		{"Pelicula en Ingles Subtitulado VOSE", VOSE},

		{"The Matrix 1999 BluRay 1080p Castellano", Castellano},
		{"El Padrino [Spanish] [1080p]", Castellano},
		{"Dune.2021.1080p.ESP", Castellano},
		{"Interstellar (2014) ES-ES", Castellano},

		// "latino" gana a "español" porque va antes en la lista.
		{"Avatar 2009 Español Latino 720p", Latino},
		{"Coco.2017.LATINO.HDRip", Latino},

		{"Oppenheimer 2023 V.O.S.E.", VOSE},
		{"Barbie 2023 subtitulada", VOSE},

		{"Gladiator 2000 DUAL Castellano/Inglés", Dual},
		{"Heat 1995 English 1080p", Ingles},

		{"Alguna Pelicula 2020 1080p x264", Desconocido},
	}
	for _, c := range casos {
		if got := DetectarIdioma(c.texto); got != c.quiero {
			t.Errorf("DetectarIdioma(%q) = %v, quiero %v", c.texto, got, c.quiero)
		}
	}
}

func TestVeredicto(t *testing.T) {
	casos := []struct {
		idioma Idioma
		quiero Veredicto
	}{
		{Castellano, Acepta},
		{Latino, Rechaza},
		{VOSE, Rechaza},
		{Ingles, Rechaza},
		{Dual, Duda},
		{Desconocido, Duda},
	}
	for _, c := range casos {
		if got := c.idioma.Veredicto(); got != c.quiero {
			t.Errorf("%v.Veredicto() = %v, quiero %v", c.idioma, got, c.quiero)
		}
	}
}

func TestDetectarCalidad(t *testing.T) {
	casos := []struct {
		texto  string
		quiero Calidad
	}{
		// Las ocho etiquetas que aparecen de verdad en EliteTorrent.
		{"720p", Cal720p},
		{"HDTV", CalHDTV},
		{"DVDrip", CalDVDRip},
		{"---", CalidadDesconocida},
		{"HQ-TS", CalTS},
		{"1080p", Cal1080p},
		{"4k", Cal4K},
		{"BRRip", CalBRRip},

		{"Peli.2021.BluRay.1080p.x264", Cal1080p},
		{"Peli 2021 WEB-DL 720p", CalWebDL},
		{"Peli 2021 MicroHD", CalMicroHD},
		{"Peli 2021 2160p HDR", Cal4K},

		// Una grabación de sala sigue siendo una grabación de sala aunque la
		// hayan subido a 1080p.
		{"Peli 2024 HQ-TS 1080p", CalTS},
		{"Peli 2024 CAM", CalCAM},

		{"Peli sin marcas", CalidadDesconocida},
	}
	for _, c := range casos {
		if got := DetectarCalidad(c.texto); got != c.quiero {
			t.Errorf("DetectarCalidad(%q) = %v, quiero %v", c.texto, got, c.quiero)
		}
	}
}

func TestRangoOrdenaComoEsperamos(t *testing.T) {
	if Cal4K.Rango() <= Cal1080p.Rango() {
		t.Error("4K debería ir por delante de 1080p")
	}
	if CalTS.Rango() >= CalDVDRip.Rango() {
		t.Error("un TS no debería ir por delante de un DVDRip")
	}
	// No saber la calidad es mejor que saber que es una grabación de sala.
	if CalidadDesconocida.Rango() <= CalCAM.Rango() {
		t.Error("lo desconocido debería ir por delante de un CAM")
	}
	if CalidadDesconocida.Rango() >= CalHDTV.Rango() {
		t.Error("lo desconocido no debería ir por delante de un HDTV")
	}
}

func TestAnalizar(t *testing.T) {
	casos := []struct {
		titulo string
		quiero Info
	}{
		{
			"The Matrix Resurrections",
			Info{Obra: "the matrix resurrections"},
		},
		{
			"Dune.Parte.Dos.2024.1080p.Castellano",
			Info{Obra: "dune parte dos", Año: 2024, Idioma: Castellano, Calidad: Cal1080p},
		},
		{
			"Los Soprano S01E05 Español Castellano HDTV",
			Info{Obra: "los soprano", Temporada: 1, Episodio: 5, Idioma: Castellano, Calidad: CalHDTV},
		},
		{
			"El Ministerio del Tiempo 2x03 [Castellano][720p]",
			Info{Obra: "el ministerio del tiempo", Temporada: 2, Episodio: 3, Idioma: Castellano, Calidad: Cal720p},
		},
		{
			"La Casa de Papel Temporada 3 Capitulo 4 Latino",
			Info{Obra: "la casa de papel", Temporada: 3, Episodio: 4, Idioma: Latino},
		},
		{
			// El año del título no debe ganarle al de publicación.
			"Blade Runner 2049 2017 BluRay Castellano",
			Info{Obra: "blade runner", Año: 2017, Idioma: Castellano, Calidad: CalBluRay},
		},
		{
			// Visto en EliteTorrent: numeran con el signo de multiplicar, no
			// con una equis.
			"El padrino de Harlem 4×1",
			Info{Obra: "el padrino de harlem", Temporada: 4, Episodio: 1},
		},
	}
	for _, c := range casos {
		got := Analizar(c.titulo)
		if got != c.quiero {
			t.Errorf("Analizar(%q)\n  = %+v\n  quiero %+v", c.titulo, got, c.quiero)
		}
	}
}

// La resolución no es un año, por mucho que "2160" caiga en el rango.
func TestResolucionNoEsAño(t *testing.T) {
	for _, s := range []string{"Peli 2160p", "Peli 1080p", "Peli 1920x1080"} {
		if got := Analizar(s).Año; got != 0 {
			t.Errorf("Analizar(%q).Año = %d, quiero 0", s, got)
		}
	}
}

// Obra es lo que permite reconocer que dos resultados de sitios distintos son
// la misma película, así que tiene que dar lo mismo escriban lo que escriban
// detrás.
func TestObraIgualEntreSitios(t *testing.T) {
	variantes := []string{
		"The Matrix 1999 BluRay 1080p Castellano",
		"The.Matrix.[1999].[Spanish].[720p].x264",
		"The Matrix (1999) DVDRip Español Castellano",
	}
	quiero := "the matrix"
	for _, v := range variantes {
		if got := Obra(v); got != quiero {
			t.Errorf("Obra(%q) = %q, quiero %q", v, got, quiero)
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
		if got := SinArticulo(entra); got != quiero {
			t.Errorf("SinArticulo(%q) = %q, quiero %q", entra, got, quiero)
		}
	}
}
