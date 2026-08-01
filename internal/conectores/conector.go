// Package conectores habla con los sitios de torrents: uno cada vez, cada uno
// con su HTML y sus manías.
//
// Un conector solo busca y parsea. No decide si un resultado vale la pena, no
// ordena y no descarga nada: eso es cosa del buscador y de la interfaz. Así
// cada sitio se puede probar solo, contra un fichero HTML guardado, sin red.
package conectores

import (
	"context"
	"time"

	"github.com/davic80/iman/internal/titulos"
)

// Resultado es un torrent encontrado en un sitio.
//
// Casi todos los campos pueden venir vacíos: cada sitio da lo que le da la
// gana. Lo único que se garantiza es Sitio, Titulo y una forma de llegar al
// contenido, sea Magnet, Torrent o Ficha.
type Resultado struct {
	Sitio  string // Qué conector lo encontró
	Titulo string // Tal cual lo escribe el sitio
	Ficha  string // URL de la página del torrent en el sitio

	// Magnet y Torrent pueden estar vacíos si el sitio solo los enseña en la
	// ficha. En ese caso se rellenan luego con Resolver.
	Magnet  string
	Torrent string

	Tamaño int64     // En bytes. 0 = no se sabe
	Fecha  time.Time // Cero = no se sabe

	// Semillas y Clientes valen -1 cuando el sitio no los publica. Cero es un
	// dato ("no hay nadie compartiendo"), que no es lo mismo que no saberlo.
	Semillas int
	Clientes int

	Info titulos.Info // Idioma, calidad, año y episodio ya deducidos
}

// SemillasConocidas dice si el sitio publica el número de semillas. Sin esto,
// ordenar por semillas pondría los sitios que no las dan al final por un dato
// que no existe.
func (r Resultado) SemillasConocidas() bool { return r.Semillas >= 0 }

// Enlace devuelve la mejor forma de llegar al torrent: el magnet si lo hay,
// si no el .torrent, y como último recurso la ficha del sitio.
func (r Resultado) Enlace() string {
	switch {
	case r.Magnet != "":
		return r.Magnet
	case r.Torrent != "":
		return r.Torrent
	default:
		return r.Ficha
	}
}

// Conector es lo que implementa cada sitio.
//
// Buscar tiene que respetar el ctx y devolver rápido: el buscador lanza todos
// los conectores a la vez con un presupuesto de tiempo común y no espera a los
// que se retrasan.
type Conector interface {
	// Nombre identifica al conector en la interfaz y en /salud.
	Nombre() string

	// Buscar consulta el sitio y devuelve lo que encuentre. Un sitio caído
	// devuelve error; un sitio vivo sin resultados devuelve lista vacía y
	// error nil, que no es lo mismo.
	Buscar(ctx context.Context, consulta string) ([]Resultado, error)
}

// Resolutor lo implementan los conectores cuyo sitio esconde el magnet en la
// ficha, que son la mayoría.
//
// Se hace en dos pasos a propósito. Resolver una búsqueda de 14 resultados
// serían 14 peticiones más a un sitio que ya limita el ritmo, y el usuario solo
// va a pinchar en uno. Así se pide la ficha cuando hace falta y no antes.
type Resolutor interface {
	// Resolver rellena Magnet y Torrent pidiendo la ficha del resultado.
	Resolver(ctx context.Context, r *Resultado) error
}
