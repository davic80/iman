package tmdb

// Los géneros llegan como números y hay que traducirlos. TMDB los publica en
// /genre/movie/list y /genre/tv/list, pero pedirlos sería una petición más (y un
// modo de fallo más) para una tabla de veinte filas que no cambia desde hace
// años. Así que van escritos aquí, y un test contra la API de verdad
// (TestVivoLosGenerosSiguenSiendoEstos) avisa el día que TMDB los mueva.
//
// Son dos tablas y no una: el mismo número significa cosas distintas en cine y
// en series. El 10759 solo existe en series, el 28 solo en cine, y el 37
// (Western) casualmente coincide en las dos.
var generosCine = map[int]string{
	28:    "Acción",
	12:    "Aventura",
	16:    "Animación",
	35:    "Comedia",
	80:    "Crimen",
	99:    "Documental",
	18:    "Drama",
	10751: "Familia",
	14:    "Fantasía",
	36:    "Historia",
	27:    "Terror",
	10402: "Música",
	9648:  "Misterio",
	10749: "Romance",
	878:   "Ciencia ficción",
	10770: "Película de TV",
	53:    "Suspense",
	10752: "Bélica",
	37:    "Western",
}

// En series, TMDB deja media tabla en inglés incluso pidiéndola en es-ES
// ("Kids", "News", "Soap", "Talk"…). Aquí se traducen: la página está en
// castellano y un género en inglés se lee como un fallo.
var generosSeries = map[int]string{
	10759: "Acción y aventura",
	16:    "Animación",
	35:    "Comedia",
	80:    "Crimen",
	99:    "Documental",
	18:    "Drama",
	10751: "Familia",
	10762: "Infantil",
	9648:  "Misterio",
	10763: "Noticias",
	10764: "Telerrealidad",
	10765: "Ciencia ficción y fantasía",
	10766: "Telenovela",
	10767: "Entrevistas",
	10768: "Bélica y política",
	37:    "Western",
}

// generos traduce los números a nombres. Los que no estén en la tabla se caen
// en silencio: es preferible enseñar un género menos que un "10770" suelto.
func generos(ids []int, serie bool) []string {
	tabla := generosCine
	if serie {
		tabla = generosSeries
	}

	var ns []string
	for _, id := range ids {
		if n, hay := tabla[id]; hay {
			ns = append(ns, n)
		}
	}
	return ns
}
