// Package novedades mantiene una lista de lo último que han subido los sitios,
// para poder verla sin buscar nada.
//
// Buscar sirve cuando ya sabes qué quieres. Esto es lo otro: entrar a ver qué
// hay. Un rondín pasa cada hora por la sección de películas de cada sitio,
// apunta lo que no había visto antes y se olvida de lo viejo. La página se sirve
// siempre de lo apuntado, nunca de la red: entrar en la portada no dispara
// ninguna petición a ningún sitio.
package novedades

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
)

// Valores por defecto del rondín. Se pueden cambiar antes de arrancarlo.
const (
	// CadaPorDefecto es cada cuánto se da una ronda. Una hora es de sobra para
	// unos sitios que suben unas pocas películas al día, y son tres peticiones.
	CadaPorDefecto = time.Hour

	// RetencionPorDefecto es cuánto tiempo se guarda una película desde que se
	// vio por primera vez. Pasada una semana ya no es una novedad.
	RetencionPorDefecto = 7 * 24 * time.Hour

	// MaximoPorDefecto es el tope de apuntes. La retención sola no basta: si un
	// sitio se vuelve loco y publica mil cosas, la portada tiene que seguir
	// siendo legible y el fichero pequeño.
	MaximoPorDefecto = 200

	// FichasPorDefecto es cuántas fichas se abren por ronda para completar los
	// apuntes nuevos. La rejilla de un sitio no trae magnet y a veces ni el
	// título entero; la ficha sí, pero cuesta una petición por película.
	FichasPorDefecto = 25

	// PlazoPorDefecto es lo que dura como mucho una ronda entera. Generoso
	// porque corre en segundo plano y el cliente espacia las peticiones.
	PlazoPorDefecto = 5 * time.Minute

	// ReintentoPorDefecto es lo que se espera cuando no contestó ni un sitio.
	// Pasa en el primer arranque con el volumen vacío: el rondín sale a la vez
	// que el vigilante de dominios y llega antes de que se sepa dónde vive cada
	// sitio. Sin esto, unos segundos de mala suerte dejan la portada vacía una
	// hora entera.
	ReintentoPorDefecto = 5 * time.Minute

	// intentosPorFicha es cuántas veces se reintenta una ficha que falla. Sin
	// tope, una ficha rota se pediría cada hora mientras siga en la lista.
	intentosPorFicha = 3
)

// Apunte es una película vista en la sección de novedades de un sitio.
type Apunte struct {
	conectores.Resultado

	// Visto es cuándo la vio Imán por primera vez, y es por lo que se ordena.
	// La fecha del sitio no vale: solo uno de los tres la publica. Esto no dice
	// cuándo se subió, dice cuándo apareció, que para una portada es lo mismo.
	Visto time.Time

	// Puesto es en qué posición venía en la rejilla del sitio. Los sitios
	// publican de más nuevo a más viejo, así que dentro de una misma ronda
	// —donde todo comparte Visto— es lo único que distingue lo de esta mañana
	// de lo del martes pasado. Sin esto la portada sale en orden alfabético.
	Puesto int

	// Resuelto indica que ya se abrió la ficha y el apunte está completo.
	Resuelto bool
	// Intentos son las veces que se ha intentado abrir la ficha sin conseguirlo.
	Intentos int
}

// Rondin recorre los sitios cada tanto y guarda lo que van subiendo.
type Rondin struct {
	sitios  []conectores.Novedoso
	log     *slog.Logger
	almacen *Almacen

	// Cada es cada cuánto se da una ronda.
	Cada time.Duration
	// Retencion es cuánto se guarda un apunte desde que se vio.
	Retencion time.Duration
	// Maximo es el tope de apuntes guardados.
	Maximo int
	// Fichas es cuántas fichas se completan por ronda.
	Fichas int
	// Plazo es lo que dura como mucho una ronda.
	Plazo time.Duration
	// Reintento es lo que se espera cuando la ronda no la contestó nadie.
	Reintento time.Duration

	mu       sync.RWMutex
	apuntes  map[string]Apunte // clave: sitio + ficha
	ultima   Ronda
	rondando bool
}

// Ronda es cómo fue la última pasada. Se enseña en la portada para no mentir
// sobre lo actual que es lo que se está viendo.
type Ronda struct {
	Cuando      time.Time
	Duracion    time.Duration
	Nuevos      int // Películas que no se habían visto nunca
	Repetidas   int // Las que ya estaban apuntadas
	Descartadas int // Las que no eran castellano
	Completadas int // Fichas abiertas para rellenar apuntes nuevos
	Fallos      []Fallo
}

// Fallo es un sitio que no contestó en la ronda.
type Fallo struct {
	Sitio string
	Error string
}

// Nuevo crea el rondín. El almacén puede ser nil: entonces la lista vive solo
// en memoria y cada despliegue empieza con la portada vacía hasta la primera
// ronda, que se da al arrancar.
func Nuevo(log *slog.Logger, almacen *Almacen, cs ...conectores.Novedoso) *Rondin {
	if log == nil {
		log = slog.Default()
	}
	return &Rondin{
		sitios:    cs,
		log:       log,
		almacen:   almacen,
		Cada:      CadaPorDefecto,
		Retencion: RetencionPorDefecto,
		Maximo:    MaximoPorDefecto,
		Fichas:    FichasPorDefecto,
		Plazo:     PlazoPorDefecto,
		Reintento: ReintentoPorDefecto,
		apuntes:   map[string]Apunte{},
	}
}

// Restaurar carga lo apuntado en despliegues anteriores, sin tocar la red.
//
// Un fichero ilegible no es motivo para no arrancar: se avisa y se sigue con la
// lista vacía, que la primera ronda llena.
func (r *Rondin) Restaurar() {
	apuntes, err := r.almacen.Cargar()
	if err != nil {
		r.log.Warn("no se pudieron recuperar las novedades guardadas", "error", err)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range apuntes {
		r.apuntes[clave(a.Resultado)] = a
	}
	r.podar()
}

// Vigilar da una ronda ahora y otra cada Cada, hasta que se cancele el ctx.
func (r *Rondin) Vigilar(ctx context.Context) {
	// malas son las rondas seguidas en las que ha fallado alguien. Es lo que
	// separa "acabo de arrancar y aún no hay dominios" de "este sitio lleva
	// toda la noche caído".
	var malas int

	espera := r.siguiente(r.Rondar(ctx), &malas)

	for {
		t := time.NewTimer(espera)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			espera = r.siguiente(r.Rondar(ctx), &malas)
		}
	}
}

// siguiente dice cuánto hay que esperar hasta la ronda que viene y lleva la
// cuenta de las rondas malas seguidas.
//
// Un sitio que no contestó deja su hueco en la portada hasta la ronda siguiente,
// y en el primer arranque eso es lo normal: el rondín sale a la vez que el
// vigilante de dominios y llega antes de que se sepa dónde vive cada sitio. Por
// eso, si falló alguno, se vuelve antes.
//
// Pero solo las primeras veces. Si sigue fallando, la espera se dobla hasta
// llegar a la ronda normal: un sitio caído —o que no se alcanza desde este
// servidor, que pasa— no se arregla porque le preguntemos más, y preguntar cada
// cinco minutos toda la noche es exactamente lo que no queremos hacerle a estos
// sitios.
func (r *Rondin) siguiente(ronda Ronda, malas *int) time.Duration {
	if len(ronda.Fallos) == 0 {
		*malas = 0
		return r.Cada
	}
	*malas++

	if r.Reintento <= 0 || r.Reintento >= r.Cada {
		return r.Cada
	}

	espera := r.Reintento
	for i := 1; i < *malas && espera < r.Cada; i++ {
		espera *= 2
	}
	if espera > r.Cada {
		espera = r.Cada
	}
	return espera
}

// Rondar da una pasada por todos los sitios y devuelve cómo fue.
//
// Si ya hay una ronda en marcha no se lanza otra: el botón de la portada y el
// reloj llaman aquí, y dos rondas a la vez serían el doble de peticiones a los
// mismos sitios para el mismo resultado.
func (r *Rondin) Rondar(ctx context.Context) Ronda {
	if !r.empezar() {
		return r.Ultima()
	}
	defer r.acabar()

	inicio := time.Now()
	ctx, cancelar := context.WithTimeout(ctx, r.Plazo)
	defer cancelar()

	ronda := Ronda{Cuando: inicio.UTC()}

	// En serie a propósito, como el vigilante de dominios: son tres sitios, no
	// corre prisa, y así el freno por dominio del cliente hace su trabajo sin
	// que se junten las peticiones.
	for _, s := range r.sitios {
		if ctx.Err() != nil {
			break
		}
		rs, err := r.pedir(ctx, s)
		if err != nil {
			r.log.Warn("un sitio no enseñó sus novedades", "sitio", s.Nombre(), "error", err)
			ronda.Fallos = append(ronda.Fallos, Fallo{Sitio: s.Nombre(), Error: err.Error()})
			continue
		}
		nuevos, repetidas, descartadas := r.apuntar(rs)
		ronda.Nuevos += nuevos
		ronda.Repetidas += repetidas
		ronda.Descartadas += descartadas
	}

	ronda.Completadas = r.completar(ctx)

	r.mu.Lock()
	r.podar()
	ronda.Duracion = time.Since(inicio)
	r.ultima = ronda
	guardar := r.listaOrdenada()
	r.mu.Unlock()

	if err := r.almacen.Guardar(guardar); err != nil {
		r.log.Warn("no se pudieron guardar las novedades", "error", err)
	}

	r.log.Info("ronda de novedades",
		"nuevos", ronda.Nuevos, "repetidas", ronda.Repetidas,
		"descartadas", ronda.Descartadas, "fichas", ronda.Completadas,
		"apuntes", len(guardar), "fallos", len(ronda.Fallos),
		"duracion", ronda.Duracion.Round(time.Millisecond))

	return ronda
}

// empezar coge el turno de ronda. Devuelve false si ya había una en marcha.
func (r *Rondin) empezar() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rondando {
		return false
	}
	r.rondando = true
	return true
}

func (r *Rondin) acabar() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rondando = false
}

// pedir llama a un sitio y para lo que salga mal.
//
// Igual que en el buscador: estos parsers viven de adivinar el HTML ajeno, y el
// día que un sitio cambie el maquetado no puede llevarse por delante el proceso
// entero, y menos desde una tarea de fondo donde no hay nadie mirando.
func (r *Rondin) pedir(ctx context.Context, s conectores.Novedoso) (rs []conectores.Resultado, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = errPanic{p}
			r.log.Error("panic pidiendo novedades", "sitio", s.Nombre(), "panic", p)
		}
	}()
	return s.Novedades(ctx)
}

// errPanic lleva el panic dentro para que se vea en la portada qué pasó, igual
// que un error normal de un sitio.
type errPanic struct{ p any }

func (e errPanic) Error() string { return fmt.Sprintf("el conector ha reventado: %v", e.p) }

// apuntar mete en la lista lo que ha devuelto un sitio.
func (r *Rondin) apuntar(rs []conectores.Resultado) (nuevos, repetidas, descartadas int) {
	ahora := time.Now().UTC()

	r.mu.Lock()
	defer r.mu.Unlock()

	var puesto int
	for _, res := range rs {
		// La portada es de castellano y solo de castellano. Aquí no hay
		// interruptor de dudosos como en la búsqueda: quien entra a ver qué hay
		// no está buscando nada concreto, y una lista con latinos y VOSE de
		// relleno no le sirve para eso.
		if res.Info.Idioma.Veredicto() != titulos.Acepta {
			descartadas++
			continue
		}

		k := clave(res)
		viejo, ok := r.apuntes[k]
		if !ok {
			r.apuntes[k] = Apunte{Resultado: res, Visto: ahora, Puesto: puesto}
			nuevos++
			puesto++
			continue
		}
		r.apuntes[k] = refrescar(viejo, res)
		repetidas++
		puesto++
	}
	return nuevos, repetidas, descartadas
}

// refrescar actualiza un apunte que ya estaba con lo que dice el sitio hoy.
//
// Visto no se toca: es la fecha por la que se ordena la portada, y que un sitio
// siga enseñando una película en su rejilla una semana después no la convierte
// en novedad otra vez. Lo que se resolvió abriendo la ficha tampoco se pisa: la
// rejilla trae menos datos que la ficha, y sería cambiar bueno por malo.
func refrescar(viejo Apunte, nuevo conectores.Resultado) Apunte {
	if !viejo.Resuelto {
		return Apunte{
			Resultado: nuevo,
			Visto:     viejo.Visto,
			Puesto:    viejo.Puesto,
			Intentos:  viejo.Intentos,
		}
	}
	viejo.Semillas = nuevo.Semillas
	viejo.Clientes = nuevo.Clientes
	if nuevo.Tamaño > 0 {
		viejo.Tamaño = nuevo.Tamaño
	}
	return viejo
}

// completar abre las fichas de unos cuantos apuntes que aún están a medias.
//
// Hace falta porque una rejilla de portada trae lo que cabe debajo de una
// carátula: en DonTorrent el título va sin acentos y en ningún sitio hay
// magnet. La ficha arregla las dos cosas, pero es una petición por película, así
// que se hacen unas pocas por ronda empezando por las más nuevas, que son las
// que se ven arriba, y el resto espera a la ronda siguiente.
func (r *Rondin) completar(ctx context.Context) int {
	pendientes := r.pendientes()

	var hechas int
	for _, a := range pendientes {
		if ctx.Err() != nil {
			break
		}
		res, err := r.resolver(ctx, a.Resultado)

		r.mu.Lock()
		actual, sigue := r.apuntes[clave(a.Resultado)]
		if sigue {
			if err != nil {
				actual.Intentos++
			} else {
				// Se conserva lo del apunte, no lo del resultado devuelto: el
				// que manda es el de la lista, que puede haber cambiado.
				actual.Resultado = res
				actual.Resuelto = true
				hechas++
			}
			r.apuntes[clave(a.Resultado)] = actual
		}
		r.mu.Unlock()

		if err != nil {
			r.log.Debug("no se pudo completar una novedad",
				"sitio", a.Sitio, "ficha", a.Ficha, "error", err)
		}
	}
	return hechas
}

// pendientes devuelve los apuntes por completar, de más nuevo a más viejo y
// como mucho los que caben en una ronda.
func (r *Rondin) pendientes() []Apunte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var ps []Apunte
	for _, a := range r.apuntes {
		if !a.Resuelto && a.Intentos < intentosPorFicha {
			ps = append(ps, a)
		}
	}
	ordenarPorFecha(ps)
	if len(ps) > r.Fichas {
		ps = ps[:r.Fichas]
	}
	return ps
}

// resolver abre la ficha de un resultado con el conector que lo encontró.
func (r *Rondin) resolver(ctx context.Context, res conectores.Resultado) (rr conectores.Resultado, err error) {
	defer func() {
		if p := recover(); p != nil {
			rr, err = res, errPanic{p}
			r.log.Error("panic resolviendo una novedad", "sitio", res.Sitio, "panic", p)
		}
	}()

	for _, s := range r.sitios {
		if s.Nombre() != res.Sitio {
			continue
		}
		resolutor, ok := s.(conectores.Resolutor)
		if !ok {
			// El sitio da el magnet en la propia rejilla; no hay nada que abrir.
			return res, nil
		}
		err := resolutor.Resolver(ctx, &res)
		return res, err
	}
	return res, errSinConector
}

// errSinConector solo puede pasar si un sitio desaparece de la lista con
// apuntes suyos todavía guardados.
var errSinConector = errors.New("el sitio ya no está conectado")

// podar tira lo viejo y lo que sobra. Hay que llamarlo con el candado cogido.
//
// Dos límites porque miden cosas distintas: la retención es "esto ya no es una
// novedad" y el máximo es "esto ya no cabe en una portada".
func (r *Rondin) podar() {
	if r.Retencion > 0 {
		corte := time.Now().UTC().Add(-r.Retencion)
		for k, a := range r.apuntes {
			if a.Visto.Before(corte) {
				delete(r.apuntes, k)
			}
		}
	}
	if r.Maximo <= 0 || len(r.apuntes) <= r.Maximo {
		return
	}
	lista := r.listaOrdenada()
	for _, a := range lista[r.Maximo:] {
		delete(r.apuntes, clave(a.Resultado))
	}
}

// Ultimos devuelve lo apuntado, de más nuevo a más viejo.
func (r *Rondin) Ultimos() []Apunte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listaOrdenada()
}

// Ultima devuelve cómo fue la última ronda.
func (r *Rondin) Ultima() Ronda {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ultima
}

// Sitios dice cuántos sitios enseñan novedades.
func (r *Rondin) Sitios() int { return len(r.sitios) }

// Rondando dice si hay una ronda en marcha. La portada lo enseña: quien acaba
// de darle al botón tiene que ver que algo está pasando, porque una ronda tarda
// más de lo que se tarda en recargar la página.
func (r *Rondin) Rondando() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rondando
}

// listaOrdenada saca los apuntes en orden. Hay que llamarla con el candado.
func (r *Rondin) listaOrdenada() []Apunte {
	lista := make([]Apunte, 0, len(r.apuntes))
	for _, a := range r.apuntes {
		lista = append(lista, a)
	}
	ordenarPorFecha(lista)
	return lista
}

// ordenarPorFecha pone lo más reciente primero.
//
// Manda cuándo lo vio Imán, no la fecha del sitio: solo uno de los tres la
// publica, y mezclar las dos daría un orden que salta según de dónde venga cada
// fila. Dentro del mismo momento —que es toda una ronda, y en la primera es
// todo— decide la fecha del sitio si la hay y, si no, el puesto que ocupaba en
// su rejilla, que es el orden en el que el propio sitio lo publicó. El título
// es el último desempate y está solo para que la lista no baile entre recargas.
func ordenarPorFecha(as []Apunte) {
	sort.Slice(as, func(i, j int) bool {
		a, b := as[i], as[j]
		if !a.Visto.Equal(b.Visto) {
			return a.Visto.After(b.Visto)
		}
		if !a.Fecha.Equal(b.Fecha) {
			return a.Fecha.After(b.Fecha)
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

// clave identifica un apunte. Sitio y ficha, porque la misma película en dos
// sitios son dos apuntes: juntarlos es cosa de la portada, no del rondín.
func clave(r conectores.Resultado) string { return r.Sitio + "\x00" + r.Ficha }
