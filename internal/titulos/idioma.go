// Package titulos saca información de los títulos de los torrents: idioma,
// calidad, año y episodio.
//
// Aquí vive la regla central de Imán: qué cuenta como castellano de España.
// Los sitios etiquetan el idioma de tres maneras distintas y ninguna es fiable
// del todo, así que el paquete distingue entre lo que sabe y lo que supone.
package titulos

import (
	"regexp"
	"strings"
)

// Idioma es el idioma de un torrent tal y como lo declara el sitio o lo sugiere
// su título.
type Idioma int

const (
	// Desconocido es que no había ninguna marca de idioma. No es lo mismo que
	// "no es castellano": es que no se sabe.
	Desconocido Idioma = iota
	Castellano         // Español de España
	Latino             // Español de Hispanoamérica
	Dual               // Dos pistas de audio, normalmente una en inglés
	VOSE               // Versión original subtitulada al español
	Ingles
)

// Veredicto es qué hacer con un resultado según su idioma.
type Veredicto int

const (
	// Rechaza: sabemos que no es castellano.
	Rechaza Veredicto = iota
	// Duda: podría serlo. Un Dual suele traer castellano, pero no siempre, y
	// un título sin marcas puede ser cualquier cosa. Se enseña aparte en vez
	// de tirarlo, porque tirarlo esconde resultados buenos.
	Duda
	// Acepta: es castellano.
	Acepta
)

func (i Idioma) Veredicto() Veredicto {
	switch i {
	case Castellano:
		return Acepta
	case Dual, Desconocido:
		return Duda
	default:
		return Rechaza
	}
}

func (i Idioma) String() string {
	switch i {
	case Castellano:
		return "castellano"
	case Latino:
		return "latino"
	case Dual:
		return "dual"
	case VOSE:
		return "VOSE"
	case Ingles:
		return "inglés"
	default:
		return "desconocido"
	}
}

// Las marcas se prueban en orden y gana la primera. El orden importa mucho:
// "Español Latino" contiene "español", e "Inglés Subtitulado VOSE" contiene
// "inglés". Lo más específico va primero.
//
// Los patrones se escriben ya normalizados: normalizar() convierte puntos,
// guiones y corchetes en espacios, así que aquí "V.O.S.E." es "v o s e".
var marcasIdioma = []struct {
	re     *regexp.Regexp
	idioma Idioma
}{
	{regexp.MustCompile(`\b(vose|vosc|subtitulad[oa]s?|subs? esp\w*|v o s e?)\b`), VOSE},
	{regexp.MustCompile(`\b(latino|latin|lat)\b`), Latino},
	{regexp.MustCompile(`\bdual\b`), Dual},
	// "cast" es ambiguo: marca de castellano, pero también una palabra inglesa
	// corriente ("Cast Away"). Se acepta el falso positivo porque es raro y
	// porque "[Cast]" es una de las marcas más usadas en los sitios españoles.
	{regexp.MustCompile(`\b(castellano|cast|espanol|spanish|esp|es es|spa)\b`), Castellano},
	{regexp.MustCompile(`\b(ingles|english|eng|v o)\b`), Ingles},
}

// DetectarIdioma adivina el idioma a partir de un texto libre: el título del
// torrent, normalmente.
//
// Es una suposición. Cuando el sitio declara el idioma en un campo aparte, hay
// que creer al sitio y no llamar a esta función.
func DetectarIdioma(s string) Idioma {
	n := normalizar(s)
	for _, m := range marcasIdioma {
		if m.re.MatchString(n) {
			return m.idioma
		}
	}
	return Desconocido
}

// Normalizar deja el texto en minúsculas, sin acentos y con los separadores
// convertidos en espacios, para que "Español", "ESPANOL" y "[espanol]" sean lo
// mismo a la hora de compararlos.
func Normalizar(s string) string { return normalizar(s) }

func normalizar(s string) string {
	s = strings.ToLower(s)
	s = sinAcentos.Replace(s)
	s = separadores.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

var sinAcentos = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
	"à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u",
	"ä", "a", "ë", "e", "ï", "i", "ö", "o", "ü", "u",
	"ñ", "n", "ç", "c",
	// Los sitios españoles numeran los episodios con el signo de multiplicar
	// ("4×1"), no con una equis. Visto en EliteTorrent.
	"×", "x",
)

var separadores = regexp.MustCompile(`[._\-\[\]()/,;:|+]+`)
