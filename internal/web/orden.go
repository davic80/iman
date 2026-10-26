package web

import (
	"net/url"
	"sort"
	"time"

	"github.com/davic80/iman/internal/buscador"
)

// Órdenes de la búsqueda. Por defecto manda el del buscador: castellano seguro
// primero, luego lo que más se parece a lo pedido, luego la calidad.
//
// Semillas no está: ninguno de los sitios las publica. Y fecha y tamaño no los
// da todo el mundo (DonTorrent no da ninguno de los dos en la lista), así que lo
// que no se sabe va al final en vez de contar como cero, que lo mezclaría con
// lo más antiguo o lo más pequeño.
const (
	ordenRelevancia = ""
	ordenFecha      = "fecha"
	ordenTamaño     = "tamano"
)

func ordenDe(q url.Values) string {
	switch o := q.Get("orden"); o {
	case ordenFecha, ordenTamaño:
		return o
	}
	return ordenRelevancia
}

// ordenarPor reordena en el sitio. Es estable: entre dos iguales, o dos que no
// dicen el dato, se queda el orden de relevancia que ya traían.
func ordenarPor(rs []buscador.Resultado, orden string) {
	switch orden {
	case ordenFecha:
		sort.SliceStable(rs, func(i, j int) bool {
			a, b := fechaDe(rs[i]), fechaDe(rs[j])
			if a.IsZero() || b.IsZero() {
				return !a.IsZero() && b.IsZero()
			}
			return a.After(b)
		})
	case ordenTamaño:
		sort.SliceStable(rs, func(i, j int) bool {
			return tamañoDe(rs[i]) > tamañoDe(rs[j])
		})
	}
}

// opcionesOrden son los botones. El activo no se puede quitar pulsándolo, como
// un filtro: siempre hay algún orden, así que se vuelve con "Relevancia".
func opcionesOrden(activo, ruta string, q url.Values) []opcion {
	todas := []opcion{
		{Valor: ordenRelevancia, Etiqueta: "Relevancia"},
		{Valor: ordenFecha, Etiqueta: "Más recientes"},
		{Valor: ordenTamaño, Etiqueta: "Más grandes"},
	}
	for i := range todas {
		o := &todas[i]
		o.Activa = o.Valor == activo
		if o.Valor == ordenRelevancia {
			o.URL = enlaceSin(ruta, q, "orden")
		} else {
			o.URL = enlaceCon(ruta, q, "orden", o.Valor)
		}
	}
	return todas
}

// fechaDe y tamañoDe miran toda la fila: es el mismo torrent en varios sitios,
// y basta con que uno diga el dato. DonTorrent no da ninguno de los dos, y si
// manda él en la fila no tiene por qué quedarse sin ellos.
func fechaDe(r buscador.Resultado) time.Time {
	if !r.Fecha.IsZero() {
		return r.Fecha
	}
	for _, o := range r.Repetidos {
		if !o.Fecha.IsZero() {
			return o.Fecha
		}
	}
	return time.Time{}
}

func tamañoDe(r buscador.Resultado) int64 {
	if r.Tamaño > 0 {
		return r.Tamaño
	}
	for _, o := range r.Repetidos {
		if o.Tamaño > 0 {
			return o.Tamaño
		}
	}
	return 0
}
