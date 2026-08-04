package conectores

import "context"

// Huella es cómo se reconoce que un dominio es de verdad el sitio que dice ser.
//
// Hace falta porque el dominio abandonado de un sitio de torrents no se queda
// vacío: lo compra un parking que responde 200 a todo y a veces imita el diseño
// del original. Aceptar un dominio por responder es aceptar al impostor.
type Huella struct {
	// Contiene son marcas que el sitio auténtico siempre lleva: un texto de la
	// plantilla, una clase CSS propia, el nombre del sitio. Tienen que estar
	// todas.
	Contiene []string

	// NoContiene son marcas de parking propias de este sitio, además de las
	// comunes de MarcasDeParking. Basta una para rechazarlo.
	NoContiene []string

	// Consulta es la búsqueda de prueba. Tiene que ser algo que ese sitio
	// seguro que tiene, y a la vez tan común que no vaya a desaparecer.
	Consulta string

	// MinAciertos es cuántos resultados parseables tiene que devolver esa
	// búsqueda. Es la verificación que no se puede falsear: un parking sabe
	// copiar un diseño, pero no sabe buscar.
	MinAciertos int
}

// MarcasDeParking son las señales de que detrás de un dominio ya no hay un
// sitio, sino alguien intentando venderlo o cobrar por la publicidad.
//
// La lista sale de diseccionar dominios abandonados de estos mismos sitios.
// Se comprueba en minúsculas.
var MarcasDeParking = []string{
	"godaddy",
	"sedoparking",
	"buy this domain",
	"comprar este dominio",
	"este dominio está en venta",
	"domain for sale",
	"sk-park.php",
	`"mode":"iframe"`,
	"parkingcrew",
	"bodis.com",
}

// Sondeo es lo que se ve al asomarse a un dominio candidato: el HTML que
// devolvió y cuántos resultados entendió el parser.
//
// Van juntos a propósito. Comprobar las marcas y probar la búsqueda son dos
// verificaciones distintas, pero salen de la misma petición: pedir dos veces al
// sitio para averiguar lo mismo es maltratarlo sin motivo.
type Sondeo struct {
	HTML       string
	Resultados int
}

// Mudable lo implementan los conectores cuyo sitio cambia de dominio, que son
// todos los públicos.
//
// El conector no busca su dominio nuevo: solo sabe reconocerse a sí mismo y
// aceptar el que le den. Quién lo busca y con qué estrategias es cosa del
// resolutor, que así vale para todos.
type Mudable interface {
	Conector
	Anfitrion

	// Base es la URL en la que se está buscando ahora.
	Base() string

	// Mudar adopta un dominio nuevo. Se llama desde el vigilante mientras
	// puede haber búsquedas en marcha, así que tiene que ser seguro.
	Mudar(base string)

	// Semillas son dominios conocidos de este sitio, empezando por el más
	// probable. Valen tanto de punto de partida como de sitio desde el que
	// seguir una redirección.
	Semillas() []string

	// Huella es cómo comprobar que un candidato es este sitio.
	Huella() Huella

	// Sondear hace la búsqueda de prueba contra una base concreta, sin cambiar
	// la que el conector esté usando.
	Sondear(ctx context.Context, base string) (Sondeo, error)
}

// ConCanal lo implementan los sitios que anuncian sus mudanzas en un canal
// público de Telegram, que es la fuente más fiable que hay cuando existe: la
// pone el propio sitio.
type ConCanal interface {
	// Canal es el nombre del canal, sin la arroba.
	Canal() string
}
