package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/novedades"
)

type datosNovedades struct {
	datosBase
	Orden           string
	PorFecha        bool
	PorSitios       bool
	Filtros         barraFiltros
	Peliculas       []vistaNovedad
	Ultima          string // Cuándo fue la última ronda, en relativo
	Rondando        bool
	Fallos          []novedades.Fallo
	SinSitios       bool
	SinNadaAun      bool
	SinNadaFiltrado bool // Hay cosas apuntadas, pero los filtros no dejan ninguna
}

// vistaNovedad es una fila de la portada lista para pintar. Reaprovecha la vista
// de la búsqueda porque es exactamente lo mismo que se enseña allí: si aquí se
// pintara distinto, el mismo torrent tendría dos caras según por dónde se entre.
type vistaNovedad struct {
	vistaResultado
	Cuando string // "hace 3 horas"
	Sitios int
}

func (s *Servidor) paginaNovedades(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	orden := novedades.Orden(q.Get("orden")).Valida()
	filas := s.novedades.Portada(orden)
	ultima := s.novedades.Ultima()

	// Los filtros se cuentan sobre todo lo apuntado y se aplican después, para
	// que los botones sigan diciendo cuánto hay detrás de cada uno.
	f := filtrosDe(q)
	todos := make([]buscador.Resultado, 0, len(filas))
	for _, fila := range filas {
		todos = append(todos, fila.Resultado)
	}

	datos := datosNovedades{
		datosBase:  s.base(),
		Orden:      string(orden),
		PorFecha:   orden == novedades.PorFecha,
		PorSitios:  orden == novedades.PorSitios,
		Filtros:    f.barra(todos, "/novedades", q),
		Peliculas:  make([]vistaNovedad, 0, len(filas)),
		Rondando:   s.novedades.Rondando(),
		Fallos:     ultima.Fallos,
		SinSitios:  s.novedades.Sitios() == 0,
		SinNadaAun: len(filas) == 0,
	}
	if !ultima.Cuando.IsZero() {
		datos.Ultima = hace(ultima.Cuando)
	}

	// Se aparta primero lo que pasa los filtros y luego se piden las carátulas
	// de eso y nada más: la portada trae setenta y pico películas y filtrar
	// después sería preguntar por las que no se van a pintar.
	var vistas []novedades.Fila
	for _, fila := range filas {
		if f.pasaCalidad(fila.Resultado) && f.pasaSitio(fila.Resultado) {
			vistas = append(vistas, fila)
		}
	}

	pintables := make([]buscador.Resultado, 0, len(vistas))
	for _, fila := range vistas {
		pintables = append(pintables, fila.Resultado)
	}
	fichas := s.fichas(r.Context(), pintables)

	for i, fila := range vistas {
		datos.Peliculas = append(datos.Peliculas, vistaNovedad{
			vistaResultado: vista(fila.Resultado, fichas[i]),
			Cuando:         hace(fila.Visto),
			Sitios:         len(fila.Sitios()),
		})
	}
	datos.SinNadaFiltrado = len(datos.Peliculas) == 0 && !datos.SinNadaAun

	s.pintar(w, r, "novedades", datos)
}

// refrescarNovedades lanza una ronda a mano, para no tener que esperar a la
// hora en punto.
//
// La ronda se va a segundo plano y la respuesta vuelve enseguida: recorrer los
// sitios lleva minutos, y dejar la petición del navegador colgada todo ese rato
// solo sirve para que se agote el tiempo de espera. Quien recargue verá el aviso
// de que hay una ronda en marcha, y sus resultados en cuanto acabe.
func (s *Servidor) refrescarNovedades(w http.ResponseWriter, r *http.Request) {
	if !s.novedades.Rondando() {
		go func() {
			// Contexto propio: el de la petición muere con la redirección, y la
			// ronda tiene que sobrevivirla. El límite lo pone el rondín.
			s.novedades.Rondar(context.Background())
		}()
	}
	// Se vuelve a la portada tal y como estaba, filtros incluidos: quien pulsa
	// "Actualizar" quiere lo mismo que estaba mirando, pero recién traído.
	vuelta := make(url.Values)
	for _, clave := range []string{"orden", "calidad", "sitio"} {
		if v := r.FormValue(clave); v != "" {
			vuelta.Set(clave, v)
		}
	}
	http.Redirect(w, r, conCola("/novedades", vuelta), http.StatusSeeOther)
}

// hace pone una fecha en palabras. La hora exacta a la que Imán vio una película
// no le importa a nadie; lo que se quiere saber es si es de hoy o de la semana
// pasada.
func hace(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "ahora mismo"
	case d < time.Hour:
		return plural(int(d.Minutes()), "hace %d minuto", "hace %d minutos")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hace %d hora", "hace %d horas")
	default:
		return plural(int(d.Hours()/24), "hace %d día", "hace %d días")
	}
}

func plural(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf(singular, n)
	}
	return fmt.Sprintf(plural, n)
}
