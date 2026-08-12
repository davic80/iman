package web

import (
	"net/url"
	"sort"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/titulos"
)

// sinCalidad es el valor que se le da a lo que no dice de qué calidad es. Sin
// él esas filas no las podría alcanzar ningún filtro de calidad, y son muchas:
// DonTorrent saca el título del nombre de la imagen y a veces no viene nada.
const sinCalidad = "ninguna"

// Filtros son los recortes que pide el usuario, y viajan en la URL para que un
// enlace con filtros puestos siga enseñando lo mismo cuando se comparte o se
// guarda.
//
// Solo se filtra por lo que los tres sitios publican de verdad. Semillas no,
// porque ninguno las da (siempre llegan a -1); tamaño tampoco, porque
// DonTorrent no lo publica nunca y DivxTotal solo a veces: un filtro de tamaño
// escondería en silencio casi todo el catálogo por no saber cuánto pesa.
type Filtros struct {
	Calidad string // "" son todas; sinCalidad, las que no lo dicen
	Sitio   string // "" son todos
}

func filtrosDe(q url.Values) Filtros {
	return Filtros{
		Calidad: q.Get("calidad"),
		Sitio:   q.Get("sitio"),
	}
}

func (f Filtros) puestos() bool { return f.Calidad != "" || f.Sitio != "" }

// pasaCalidad y pasaSitio van sueltas porque para contar cuántos resultados
// dejaría cada opción hay que aplicar los demás filtros pero no el suyo: si no,
// todos los números saldrían iguales a lo que ya se está viendo.
func (f Filtros) pasaCalidad(r buscador.Resultado) bool {
	switch f.Calidad {
	case "":
		return true
	case sinCalidad:
		return r.Info.Calidad == titulos.CalidadDesconocida
	default:
		return string(r.Info.Calidad) == f.Calidad
	}
}

// pasaSitio mira todos los sitios de la fila, no solo el que manda: una fila
// fundida es la misma película en varios sitios, y si se pide DonTorrent hay
// que enseñarla también cuando DonTorrent es uno de los repetidos.
func (f Filtros) pasaSitio(r buscador.Resultado) bool {
	if f.Sitio == "" {
		return true
	}
	for _, s := range r.Sitios() {
		if s == f.Sitio {
			return true
		}
	}
	return false
}

func (f Filtros) aplicar(rs []buscador.Resultado) []buscador.Resultado {
	if !f.puestos() {
		return rs
	}
	out := make([]buscador.Resultado, 0, len(rs))
	for _, r := range rs {
		if f.pasaCalidad(r) && f.pasaSitio(r) {
			out = append(out, r)
		}
	}
	return out
}

// opcion es un botón de la barra de filtros, ya con su enlace hecho.
//
// Valor es lo que viaja en la URL y Etiqueta lo que se lee: coinciden en todo
// menos en las que no dicen su calidad, donde el valor tiene que ser una
// palabra sin acentos y lo que se enseña, castellano.
type opcion struct {
	Valor    string
	Etiqueta string
	Cuantos  int
	URL      string
	Activa   bool
}

func etiquetaDe(valor string) string {
	if valor == sinCalidad {
		return "Sin calidad"
	}
	return valor
}

// barraFiltros es lo que pinta la plantilla. Solo se enseñan las opciones que
// existen en lo que hay delante: no tiene sentido ofrecer "4K" en una lista
// donde no hay ni un 4K, ni ofrecer un sitio cuando solo contesta uno.
type barraFiltros struct {
	Calidades []opcion
	Sitios    []opcion
	Puestos   bool
	Total     int    // cuántos había antes de filtrar
	URLLimpia string // para quitarlos todos

	// Lo que hay puesto, tal cual, para que el formulario de actualizar la
	// portada lo pueda devolver y no se pierdan al recargar.
	Calidad string
	Sitio   string
}

// barra arma los botones a partir de los resultados sin filtrar.
//
// ruta y q son de dónde se ha entrado: los enlaces conservan lo que ya
// hubiera puesto (la consulta, el orden de la portada) y solo cambian el filtro
// que toca, porque un filtro que te borra la búsqueda no lo usa nadie dos veces.
func (f Filtros) barra(rs []buscador.Resultado, ruta string, q url.Values) barraFiltros {
	b := barraFiltros{
		Puestos:   f.puestos(),
		Total:     len(rs),
		URLLimpia: enlaceSin(ruta, q, "calidad", "sitio"),
		Calidad:   f.Calidad,
		Sitio:     f.Sitio,
	}

	// Cada lista se cuenta con los demás filtros aplicados y el suyo no.
	porCalidad := make(map[string]int)
	porSitio := make(map[string]int)
	for _, r := range rs {
		if f.pasaSitio(r) {
			cal := string(r.Info.Calidad)
			if cal == "" {
				cal = sinCalidad
			}
			porCalidad[cal]++
		}
		if f.pasaCalidad(r) {
			// Una fila fundida cuenta una vez por cada sitio que la tiene: si no,
			// el número del botón no cuadraría con lo que se enseña al pulsarlo.
			for _, s := range sinRepetir(r.Sitios()) {
				porSitio[s]++
			}
		}
	}

	b.Calidades = opciones(porCalidad, f.Calidad, ruta, q, "calidad")
	b.Sitios = opciones(porSitio, f.Sitio, ruta, q, "sitio")

	// De mejor a peor, y lo que no dice qué calidad tiene, al final.
	sort.SliceStable(b.Calidades, func(i, j int) bool {
		a, c := b.Calidades[i], b.Calidades[j]
		if a.Valor == sinCalidad || c.Valor == sinCalidad {
			return c.Valor == sinCalidad && a.Valor != sinCalidad
		}
		return titulos.Calidad(a.Valor).Rango() > titulos.Calidad(c.Valor).Rango()
	})
	sort.SliceStable(b.Sitios, func(i, j int) bool {
		return b.Sitios[i].Valor < b.Sitios[j].Valor
	})

	return b
}

// opciones convierte el recuento en botones. Uno solo no es una elección, así
// que si solo hay un valor posible y nadie ha filtrado, no se enseña nada.
func opciones(cuenta map[string]int, activo, ruta string, q url.Values, clave string) []opcion {
	if len(cuenta) < 2 && activo == "" {
		return nil
	}

	out := make([]opcion, 0, len(cuenta))
	for valor, n := range cuenta {
		o := opcion{Valor: valor, Etiqueta: etiquetaDe(valor), Cuantos: n, Activa: valor == activo}
		// Volver a pulsar el que está puesto lo quita: es el gesto que espera
		// cualquiera y ahorra un botón de "quitar" por filtro.
		if o.Activa {
			o.URL = enlaceSin(ruta, q, clave)
		} else {
			o.URL = enlaceCon(ruta, q, clave, o.Valor)
		}
		out = append(out, o)
	}
	return out
}

func enlaceCon(ruta string, q url.Values, clave, valor string) string {
	otra := copiar(q)
	otra.Set(clave, valor)
	return conCola(ruta, otra)
}

func enlaceSin(ruta string, q url.Values, claves ...string) string {
	otra := copiar(q)
	for _, c := range claves {
		otra.Del(c)
	}
	return conCola(ruta, otra)
}

func conCola(ruta string, q url.Values) string {
	if len(q) == 0 {
		return ruta
	}
	return ruta + "?" + q.Encode()
}

func copiar(q url.Values) url.Values {
	otra := make(url.Values, len(q))
	for k, v := range q {
		otra[k] = append([]string(nil), v...)
	}
	return otra
}

func sinRepetir(ss []string) []string {
	vistos := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !vistos[s] {
			vistos[s] = true
			out = append(out, s)
		}
	}
	return out
}
