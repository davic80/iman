package novedades

import (
	"sort"
	"time"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/conectores"
)

// Orden es cómo se quiere ver la portada.
type Orden string

const (
	// PorFecha pone arriba lo último que apareció. Es lo que se quiere casi
	// siempre: entrar a ver qué hay de nuevo desde ayer.
	PorFecha Orden = "fecha"

	// PorSitios pone arriba lo que han subido más sitios. Con tres sitios no da
	// para un ranking, pero sí distingue el estreno que ha publicado todo el
	// mundo del relleno que solo tiene uno.
	PorSitios Orden = "sitios"
)

// Valida devuelve un orden que se pueda usar. Lo que llegue raro por la URL cae
// en el de siempre en vez de dar un error por algo que no importa tanto.
func (o Orden) Valida() Orden {
	if o == PorSitios {
		return PorSitios
	}
	return PorFecha
}

// Fila es una película de la portada: juntas ya todas las veces que se ha
// publicado, en el sitio que sea.
type Fila struct {
	buscador.Resultado

	// Visto es cuándo apareció por primera vez en cualquiera de los sitios que
	// la tienen. Manda el más madrugador: una película no vuelve a ser novedad
	// porque un segundo sitio la suba tres días después.
	Visto time.Time

	// Puesto es lo más alto que ha llegado en la rejilla de algún sitio. Sirve
	// para desempatar dentro de una misma ronda, donde el Visto es el mismo
	// para todo.
	Puesto int
}

// Portada devuelve lo apuntado, agrupado por película y en el orden que se pida.
//
// Se agrupa con el mismo Fundir que la búsqueda: si aquí se juntaran las
// películas con otro criterio, los mismos datos saldrían agrupados de dos
// maneras distintas según por dónde se entre.
func (r *Rondin) Portada(orden Orden) []Fila {
	apuntes := r.Ultimos()

	crudos := make([]conectores.Resultado, 0, len(apuntes))
	porClave := make(map[string]Apunte, len(apuntes))
	for _, a := range apuntes {
		crudos = append(crudos, a.Resultado)
		porClave[clave(a.Resultado)] = a
	}

	fundidos := buscador.Fundir(crudos)
	filas := make([]Fila, 0, len(fundidos))
	for _, f := range fundidos {
		visto, puesto := primeraVez(f, porClave)
		filas = append(filas, Fila{Resultado: f, Visto: visto, Puesto: puesto})
	}

	ordenarFilas(filas, orden.Valida())
	return filas
}

// primeraVez resume en un solo par lo que dicen todas las publicaciones que se
// han juntado en una fila: cuándo se vio la primera y lo más alto que llegó en
// la rejilla de alguna de ellas.
func primeraVez(f buscador.Resultado, porClave map[string]Apunte) (time.Time, int) {
	a := porClave[clave(f.Resultado)]
	visto, puesto := a.Visto, a.Puesto

	for _, otro := range f.Repetidos {
		o := porClave[clave(otro)]
		if visto.IsZero() || (!o.Visto.IsZero() && o.Visto.Before(visto)) {
			visto = o.Visto
		}
		if o.Puesto < puesto {
			puesto = o.Puesto
		}
	}
	return visto, puesto
}

func ordenarFilas(fs []Fila, orden Orden) {
	sort.Slice(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]

		if orden == PorSitios {
			if na, nb := len(a.Sitios()), len(b.Sitios()); na != nb {
				return na > nb
			}
		}
		if !a.Visto.Equal(b.Visto) {
			return a.Visto.After(b.Visto)
		}
		// Dentro del mismo momento —y todo lo de una misma ronda comparte
		// momento— delante lo que tienen más sitios y lo que más arriba salía en
		// su rejilla. El título es el último desempate, para que la lista no
		// baile entre dos recargas iguales.
		if na, nb := len(a.Sitios()), len(b.Sitios()); na != nb {
			return na > nb
		}
		if a.Puesto != b.Puesto {
			return a.Puesto < b.Puesto
		}
		if a.Titulo != b.Titulo {
			return a.Titulo < b.Titulo
		}
		return a.Sitio < b.Sitio
	})
}
