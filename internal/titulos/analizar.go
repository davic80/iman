package titulos

import (
	"regexp"
	"strconv"
	"strings"
)

// Info es todo lo que se puede deducir del título de un torrent.
//
// Los ceros significan "no se sabe": ni año, ni temporada, ni episodio son
// obligatorios. Una película no tiene temporada y muchos títulos no traen año.
type Info struct {
	Obra      string // Título sin marcas, para agrupar versiones de lo mismo
	Año       int
	Temporada int
	Episodio  int
	Idioma    Idioma
	Calidad   Calidad
}

// EsSerie indica si el título trae numeración de episodio o temporada.
func (i Info) EsSerie() bool { return i.Temporada > 0 || i.Episodio > 0 }

// Analizar deduce todo lo que puede de un título suelto.
//
// Se usa cuando el sitio solo da una cadena. Si el sitio declara el idioma o la
// calidad por separado hay que creerle a él y sobrescribir estos campos: un
// campo declarado siempre vale más que una suposición sobre el texto.
func Analizar(titulo string) Info {
	n := normalizar(titulo)
	temp, ep := detectarEpisodio(n)
	return Info{
		Obra:      Obra(titulo),
		Año:       detectarAño(n),
		Temporada: temp,
		Episodio:  ep,
		Idioma:    DetectarIdioma(titulo),
		Calidad:   DetectarCalidad(titulo),
	}
}

var reAño = regexp.MustCompile(`\b(19|20)\d{2}\b`)

// detectarAño busca un año entre 1900 y el futuro cercano. Los marcadores de
// resolución no estorban: "1080p" y "2160p" llevan la "p" pegada, así que no
// hay frontera de palabra y no cuelan como año.
func detectarAño(n string) int {
	// Se coge el último: en "Blade Runner 2049 2017" el año de publicación va
	// detrás y el otro forma parte del título.
	todos := reAño.FindAllString(n, -1)
	if len(todos) == 0 {
		return 0
	}
	a, _ := strconv.Atoi(todos[len(todos)-1])
	return a
}

var (
	reTempEp   = regexp.MustCompile(`\bs(\d{1,2})e(\d{1,3})\b`)
	reTempEpX  = regexp.MustCompile(`\b(\d{1,2})x(\d{1,3})\b`)
	reTempSola = regexp.MustCompile(`\b(?:temporada|temp|season|s)\s*(\d{1,2})\b`)
	reCapitulo = regexp.MustCompile(`\b(?:capitulo|cap|episodio|ep)\s*(\d{1,3})\b`)
)

// detectarEpisodio entiende las tres formas que usan los sitios españoles:
// "S01E02", "1x02" y "Temporada 1 Capitulo 2".
func detectarEpisodio(n string) (temporada, episodio int) {
	if m := reTempEp.FindStringSubmatch(n); m != nil {
		t, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		return t, e
	}
	if m := reTempEpX.FindStringSubmatch(n); m != nil {
		t, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		return t, e
	}
	if m := reTempSola.FindStringSubmatch(n); m != nil {
		temporada, _ = strconv.Atoi(m[1])
	}
	if m := reCapitulo.FindStringSubmatch(n); m != nil {
		episodio, _ = strconv.Atoi(m[1])
	}
	return temporada, episodio
}

// cortes son las marcas que indican que el título de la obra ya se acabó y
// empiezan los detalles de la release.
var cortes = regexp.MustCompile(
	`\b(19\d{2}|20\d{2}|s\d{1,2}e\d{1,3}|\d{1,2}x\d{1,3}|temporada|temp|season|capitulo|` +
		`castellano|espanol|spanish|latino|dual|vose|subtitulado|ingles|english|` +
		`4k|2160p|1080p?|720p?|480p|microhd|bluray|blu ray|bdrip|brrip|web ?dl|webrip|` +
		`hdrip|hdtv|dvdrip|dvd|screener|hq ?ts|hdts|telesync|x264|x265|h264|h265|hevc|` +
		`ac3|aac|dts|mkv|avi|mp4)\b`)

// Obra devuelve el título sin los detalles de la release, para poder reconocer
// que dos resultados de sitios distintos son la misma película.
//
// Corta por la primera marca reconocida en vez de ir quitándolas una a una:
// todo lo que viene después de un año o de una calidad son detalles, y así el
// resultado no depende de en qué orden los escribiera cada sitio.
func Obra(titulo string) string {
	n := normalizar(titulo)
	if loc := cortes.FindStringIndex(n); loc != nil {
		n = n[:loc[0]]
	}
	return strings.TrimSpace(n)
}

// SinArticulo quita el artículo inicial de una obra ya normalizada.
//
// Es la diferencia entre "The Matrix Resurrections" de un sitio y "Matrix
// Resurrections" del otro, que son la misma película. Solo vale para comparar
// obras entre sí: lo que se enseña es el título que escribió el sitio, y la
// búsqueda mira el original, que si no "the matrix" no encontraría nada.
func SinArticulo(obra string) string {
	for _, art := range articulos {
		if resto := strings.TrimPrefix(obra, art); resto != obra {
			return resto
		}
	}
	return obra
}

var articulos = [...]string{"the ", "el ", "la ", "los ", "las ", "un ", "una "}
