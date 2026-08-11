// Package buscador lanza la misma consulta a todos los sitios a la vez y junta
// lo que devuelvan.
//
// La regla de la casa es que ningún sitio puede estropear la búsqueda de los
// demás: ni tardando, ni fallando, ni reventando. Un sitio lento se queda
// fuera cuando se acaba el presupuesto de tiempo y los otros contestan igual.
package buscador

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
)

// Buscador reparte la consulta entre los conectores y lleva la cuenta de cómo
// se porta cada uno.
type Buscador struct {
	conectores  []conectores.Conector
	presupuesto time.Duration
	log         *slog.Logger

	mu     sync.Mutex
	salud  map[string]*Salud
	numero int // Búsquedas hechas, para /salud
}

// Opciones son las preferencias de una búsqueda concreta.
type Opciones struct {
	// Dudosos incluye los resultados cuyo idioma no está claro: los duales y
	// los que no traen marca ninguna. Por defecto se dejan fuera, porque el
	// sentido de Imán es no tener que abrir cinco fichas para descubrir que
	// ninguna está en castellano.
	Dudosos bool
}

// Busqueda es el resultado de consultar a todos los sitios.
//
// No hay error: que un sitio se caiga no invalida lo que digan los demás. Los
// que fallaron se listan en Fallos para poder enseñarlo sin mentir sobre lo
// completa que es la respuesta.
type Busqueda struct {
	Consulta    string
	Resultados  []Resultado
	Descartados int // Cuántos se filtraron por idioma
	Fallos      []Fallo
	Duracion    time.Duration
}

// Fallo es un sitio que no contestó.
type Fallo struct {
	Sitio string
	Error string
}

// Completa dice si contestaron todos los sitios. Si no, los resultados son un
// trozo de la verdad y la interfaz debería decirlo.
func (b Busqueda) Completa() bool { return len(b.Fallos) == 0 }

// Nuevo crea un buscador. El presupuesto es lo máximo que se espera a que
// contesten todos: pasado ese tiempo se devuelve lo que haya llegado.
func Nuevo(log *slog.Logger, presupuesto time.Duration, cs ...conectores.Conector) *Buscador {
	if log == nil {
		log = slog.Default()
	}
	b := &Buscador{
		conectores:  cs,
		presupuesto: presupuesto,
		log:         log,
		salud:       make(map[string]*Salud, len(cs)),
	}
	for _, c := range cs {
		b.salud[c.Nombre()] = &Salud{Nombre: c.Nombre()}
	}
	return b
}

// Conectores devuelve cuántos sitios hay conectados.
func (b *Buscador) Conectores() int { return len(b.conectores) }

// Buscar pregunta a todos los sitios a la vez y devuelve lo que llegue a
// tiempo, ya filtrado y ordenado.
func (b *Buscador) Buscar(ctx context.Context, consulta string, op Opciones) Busqueda {
	inicio := time.Now()

	// El presupuesto es común a todos: lo que importa es cuánto espera el
	// usuario, no cuánto tarda cada sitio por separado.
	ctx, cancelar := context.WithTimeout(ctx, b.presupuesto)
	defer cancelar()

	var (
		mu     sync.Mutex
		crudos []conectores.Resultado
		fallos []Fallo
		grupo  sync.WaitGroup
	)

	for _, c := range b.conectores {
		grupo.Add(1)
		go func(c conectores.Conector) {
			defer grupo.Done()

			rs, err := b.consultar(ctx, c, consulta)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fallos = append(fallos, Fallo{Sitio: c.Nombre(), Error: err.Error()})
				return
			}
			crudos = append(crudos, rs...)
		}(c)
	}
	grupo.Wait()

	// Se filtra antes de fundir: un resultado en latino no tiene por qué entrar
	// en una fila y arrastrar a ella un sitio que no se va a poder enseñar.
	utiles, descartados := filtrar(crudos, op)
	filas := Fundir(utiles)
	ordenar(filas, consulta)

	// Los fallos van siempre en el mismo orden: si no, /salud y la página de
	// resultados bailan entre recargas sin que haya cambiado nada.
	sort.Slice(fallos, func(i, j int) bool { return fallos[i].Sitio < fallos[j].Sitio })

	b.mu.Lock()
	b.numero++
	b.mu.Unlock()

	return Busqueda{
		Consulta:    consulta,
		Resultados:  filas,
		Descartados: descartados,
		Fallos:      fallos,
		Duracion:    time.Since(inicio),
	}
}

// consultar llama a un conector y apunta cómo le ha ido.
//
// Aquí se para todo lo que pueda salir mal en un sitio: un error, un plantón o
// un panic dentro del parser. Estos parsers viven de adivinar el HTML ajeno, y
// el día que un sitio cambie el maquetado no puede llevarse por delante el
// proceso entero.
func (b *Buscador) consultar(ctx context.Context, c conectores.Conector, consulta string) (rs []conectores.Resultado, err error) {
	inicio := time.Now()

	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("el conector ha reventado: %v", p)
			b.log.Error("panic en un conector", "sitio", c.Nombre(), "panic", p)
		}
		b.apuntar(c.Nombre(), time.Since(inicio), len(rs), err)
	}()

	rs, err = c.Buscar(ctx, consulta)
	if err != nil {
		b.log.Warn("un sitio no contestó", "sitio", c.Nombre(), "error", err)
	}
	return rs, err
}

// filtrar quita lo que no está en castellano y cuenta cuánto se ha quitado,
// que es un dato que el usuario quiere ver: sin él, una búsqueda vacía no se
// distingue de una búsqueda cuyos resultados eran todos latinos.
func filtrar(rs []conectores.Resultado, op Opciones) (utiles []conectores.Resultado, descartados int) {
	for _, r := range rs {
		switch r.Info.Idioma.Veredicto() {
		case titulos.Acepta:
			utiles = append(utiles, r)
		case titulos.Duda:
			if op.Dudosos {
				utiles = append(utiles, r)
			} else {
				descartados++
			}
		default:
			descartados++
		}
	}
	return utiles, descartados
}

// ordenar pone delante lo que el usuario querría ver primero.
func ordenar(rs []Resultado, consulta string) {
	// La relevancia se calcula una vez por resultado y no dentro de la
	// comparación, que se ejecuta unas cuantas veces por elemento.
	puntos := make(map[string]int, len(rs))
	for _, r := range rs {
		puntos[r.Sitio+"\x00"+r.Ficha] = relevancia(consulta, r.Resultado)
	}
	puntuacion := func(r Resultado) int { return puntos[r.Sitio+"\x00"+r.Ficha] }

	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]

		// El castellano seguro va antes que el dudoso, siempre.
		if va, vb := a.Info.Idioma.Veredicto(), b.Info.Idioma.Veredicto(); va != vb {
			return va > vb
		}
		// Y la relevancia antes que la calidad: un 1080p de otra película no
		// le sirve a nadie. Los sitios buscan por palabras sueltas y devuelven
		// mucho ruido, así que sin esto lo que sale arriba es lo que mejor se
		// ve, no lo que se buscaba.
		if pa, pb := puntuacion(a), puntuacion(b); pa != pb {
			return pa > pb
		}
		// Dentro de una misma serie manda el orden de emisión. Ordenar los
		// capítulos por calidad deja una lista imposible de leer (6x9, 6x5,
		// 6x10) cuando lo que se busca es el siguiente que falta.
		if mismaSerie(a.Info, b.Info) {
			if a.Info.Temporada != b.Info.Temporada {
				return a.Info.Temporada < b.Info.Temporada
			}
			return a.Info.Episodio < b.Info.Episodio
		}

		if ra, rb := a.Info.Calidad.Rango(), b.Info.Calidad.Rango(); ra != rb {
			return ra > rb
		}
		// Entre dos calidades iguales manda quién tiene más semillas, pero un
		// sitio que no las publica no puede perder por un dato que no existe:
		// se queda a la par y decide el desempate siguiente.
		if a.SemillasConocidas() && b.SemillasConocidas() && a.Semillas != b.Semillas {
			return a.Semillas > b.Semillas
		}
		if a.Tamaño != b.Tamaño {
			return a.Tamaño > b.Tamaño
		}
		return a.Titulo < b.Titulo
	})
}

// mismaSerie dice si dos resultados son capítulos distintos de la misma serie.
// Con la obra vacía no se puede afirmar, y sin obra común serían capítulos de
// series diferentes que se ordenarían entre sí sin sentido.
func mismaSerie(a, b titulos.Info) bool {
	if !a.EsSerie() || !b.EsSerie() || a.Obra == "" || a.Obra != b.Obra {
		return false
	}
	return a.Temporada != b.Temporada || a.Episodio != b.Episodio
}

// relevancia puntúa de 0 a 100 cuánto se parece un resultado a lo que se pidió.
//
// Hace falta porque estos sitios buscan por palabras sueltas: pedir "el
// padrino" devuelve también "El guru de las bodas", porque lleva un "el". Si se
// ordena solo por calidad, lo primero que se ve es el ruido en 1080p.
func relevancia(consulta string, r conectores.Resultado) int {
	pedidas := strings.Fields(titulos.Normalizar(consulta))
	if len(pedidas) == 0 {
		return 0
	}

	titulo := titulos.Normalizar(r.Titulo)
	hay := make(map[string]bool)
	for _, p := range strings.Fields(titulo) {
		hay[p] = true
	}

	var aciertos int
	for _, p := range pedidas {
		if hay[p] {
			aciertos++
		}
	}
	puntos := aciertos * 100 / len(pedidas)

	// Que la obra sea exactamente lo que se pidió es la señal más fuerte que
	// hay: "El Padrino" por delante de "El último padrino".
	if r.Info.Obra != "" && r.Info.Obra == titulos.Normalizar(consulta) {
		puntos += 100
	}
	return puntos
}
