package buscador

import (
	"sort"
	"time"

	"github.com/davic80/iman/internal/conectores"
)

// Estos sitios cambian de dominio y de maquetado sin avisar a nadie. Cuando un
// conector deja de devolver resultados hay que poder verlo, y verlo antes de
// que lo vea el usuario en forma de búsqueda vacía. De ahí este registro.
const (
	SinDatos  = "sin datos" // Todavía no se le ha preguntado nada
	Vivo      = "vivo"
	Degradado = "degradado"
	Caido     = "caido"
)

// fallosParaCaido es cuántos fallos seguidos hacen falta para dar un sitio por
// caído. Uno solo no basta: estos sitios dan timeouts sueltos con normalidad y
// pintarlos de rojo por eso sería enseñar alarmas que no significan nada.
const fallosParaCaido = 3

// Salud es cómo se está portando un conector.
type Salud struct {
	Nombre     string
	Dominio    string // En qué dominio está buscando hoy, si el conector lo dice
	Consultas  int
	Fallos     int // Seguidos, no en total: se reinician al primer acierto
	UltimaVez  time.Time
	Latencia   time.Duration
	Resultados int    // De la última consulta que salió bien
	Error      string // El último, si lo hubo
}

// Estado resume la salud en una palabra.
func (s Salud) Estado() string {
	switch {
	case s.Consultas == 0:
		return SinDatos
	case s.Fallos == 0:
		return Vivo
	case s.Fallos < fallosParaCaido:
		return Degradado
	default:
		return Caido
	}
}

// apuntar registra cómo ha ido una consulta a un sitio.
func (b *Buscador) apuntar(nombre string, latencia time.Duration, resultados int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, hay := b.salud[nombre]
	if !hay {
		s = &Salud{Nombre: nombre}
		b.salud[nombre] = s
	}

	s.Consultas++
	s.UltimaVez = time.Now()
	s.Latencia = latencia

	if err != nil {
		s.Fallos++
		s.Error = err.Error()
		return
	}
	// Un acierto borra la cuenta de fallos: lo que interesa es si el sitio
	// está roto ahora, no cuántas veces ha fallado desde que arrancó.
	s.Fallos = 0
	s.Error = ""
	s.Resultados = resultados
}

// Salud devuelve el estado de todos los conectores, en orden fijo para que
// /salud no baile entre recargas.
func (b *Buscador) Salud() []Salud {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]Salud, 0, len(b.salud))
	for _, s := range b.salud {
		copia := *s
		// El dominio se pregunta al vuelo y no se guarda: cambia cuando el
		// resolutor descubre que el sitio se ha mudado, y lo que interesa en
		// /salud es dónde se está buscando ahora mismo.
		if a, sabe := b.conector(copia.Nombre).(conectores.Anfitrion); sabe {
			copia.Dominio = a.Dominio()
		}
		out = append(out, copia)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nombre < out[j].Nombre })
	return out
}

// Busquedas es cuántas búsquedas se han hecho desde que arrancó.
func (b *Buscador) Busquedas() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.numero
}
