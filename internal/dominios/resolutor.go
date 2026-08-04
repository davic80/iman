package dominios

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/davic80/iman/internal/conectores"
)

// Valores por defecto del resolutor. Se pueden cambiar antes de arrancarlo.
const (
	// RevalidacionPorDefecto es cada cuánto se comprueba que el dominio en uso
	// sigue siendo el bueno. Seis horas es de sobra: estos sitios se mudan cada
	// semanas, y comprobarlo cuesta dos peticiones.
	RevalidacionPorDefecto = 6 * time.Hour

	// SaltosContadorPorDefecto es cuántos números se prueban hacia delante en
	// los sitios que numeran el subdominio.
	SaltosContadorPorDefecto = 3

	// PlazoPorDefecto es lo que se le da a una ronda entera de descubrimiento.
	// Es generoso a propósito: corre en segundo plano y el cliente espacia las
	// peticiones, así que probar diez candidatos lleva su tiempo.
	PlazoPorDefecto = 5 * time.Minute
)

// Resolutor mantiene a cada conector apuntando al dominio en el que su sitio
// vive hoy.
//
// Trabaja siempre en segundo plano. Una búsqueda de un usuario nunca espera a
// una verificación: si el dominio se ha caído, esa búsqueda falla y es el
// vigilante quien arregla el sitio para la siguiente.
type Resolutor struct {
	cliente Cliente
	log     *slog.Logger
	estado  *Estado

	// Revalidacion es cada cuánto se comprueba un dominio que va bien.
	Revalidacion time.Duration
	// SaltosContador es cuántos números se prueban en los sitios numerados.
	SaltosContador int
	// Plazo es lo que dura como mucho una ronda de descubrimiento.
	Plazo time.Duration

	mu     sync.Mutex
	sitios map[string]conectores.Mudable
	ultima map[string]Comprobacion
}

// Comprobacion es cómo fue la última vez que se miró un sitio. Se enseña en
// /salud, que es donde se ve que un sitio se ha mudado.
type Comprobacion struct {
	Base       string    // Dominio vigente tras la comprobación
	Cuando     time.Time // Cuándo se comprobó
	Origen     string    // Por qué camino se encontró
	Error      string    // Vacío si fue bien
	Candidatos int       // Cuántos hubo que probar
}

// Nuevo crea el resolutor. El estado puede ser nil: entonces no se recuerda
// nada entre arranques, que funciona igual pero cuesta una ronda de
// descubrimiento cada vez que se despliega.
func Nuevo(cliente Cliente, log *slog.Logger, estado *Estado, cs ...conectores.Mudable) *Resolutor {
	if log == nil {
		log = slog.Default()
	}
	r := &Resolutor{
		cliente:        cliente,
		log:            log,
		estado:         estado,
		Revalidacion:   RevalidacionPorDefecto,
		SaltosContador: SaltosContadorPorDefecto,
		Plazo:          PlazoPorDefecto,
		sitios:         make(map[string]conectores.Mudable, len(cs)),
		ultima:         make(map[string]Comprobacion, len(cs)),
	}
	for _, c := range cs {
		r.sitios[c.Nombre()] = c
	}
	return r
}

// Restaurar pone a cada conector en el último dominio que se supo bueno, sin
// tocar la red.
//
// Se llama al arrancar. No verifica nada: verificar cuesta peticiones y el
// arranque no es momento de hacer esperar a nadie. Si el dominio guardado ya no
// vale, el vigilante lo descubrirá en su primera ronda.
func (r *Resolutor) Restaurar() {
	if r.estado == nil {
		return
	}
	for nombre, c := range r.sitios {
		guardado, ok := r.estado.Dominio(nombre)
		if !ok || normalizarBase(guardado) == "" {
			continue
		}
		if guardado != c.Base() {
			r.log.Info("dominio restaurado del estado",
				"sitio", nombre, "dominio", anfitrion(guardado))
		}
		c.Mudar(guardado)
	}
}

// Vigilar revalida los dominios cada cierto tiempo hasta que se cancele el
// contexto. Está pensado para lanzarlo en una goroutine desde main.
//
// Hace una ronda nada más arrancar. Es el momento en el que más probable es que
// algo haya cambiado, porque un despliegue puede llevar semanas de diferencia
// con el anterior.
func (r *Resolutor) Vigilar(ctx context.Context) {
	r.Revisar(ctx)

	t := time.NewTicker(r.Revalidacion)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.Revisar(ctx)
		}
	}
}

// Revisar comprueba todos los sitios, uno tras otro.
//
// En serie a propósito: son pocos sitios, no corre prisa y así no se lanzan
// peticiones a media internet a la vez cada seis horas.
func (r *Resolutor) Revisar(ctx context.Context) {
	for _, c := range r.mudables() {
		if ctx.Err() != nil {
			return
		}
		if err := r.Revisar1(ctx, c); err != nil {
			r.log.Warn("no se pudo verificar el sitio",
				"sitio", c.Nombre(), "error", err)
		}
	}
}

// Revisar1 comprueba un sitio y, si hace falta, lo muda.
//
// El dominio nuevo se adopta solo, sin preguntar, en cuanto pasa las tres
// verificaciones. Es la única forma de que esto sirva de algo: cuando un sitio
// se muda un domingo por la noche, nadie va a estar mirando.
func (r *Resolutor) Revisar1(ctx context.Context, c conectores.Mudable) error {
	ctx, cancelar := context.WithTimeout(ctx, r.Plazo)
	defer cancelar()

	actual := c.Base()

	// El que ya está en uso primero y aparte: si sigue bien, esto acaba en una
	// petición y no hay nada más que hacer.
	if _, err := r.verificar(ctx, c, actual); err == nil {
		r.apuntar(c.Nombre(), Comprobacion{Base: actual, Cuando: time.Now(), Origen: "en uso"})
		r.recordar(c.Nombre(), actual)
		return nil
	} else {
		r.log.Warn("el dominio en uso ya no vale, buscando",
			"sitio", c.Nombre(), "dominio", anfitrion(actual), "motivo", err)
	}

	cs := r.candidatos(ctx, c, actual)
	var fallos []string
	for _, cand := range cs {
		if cand.Base == actual {
			continue // Ya se ha probado arriba
		}
		if ctx.Err() != nil {
			break
		}
		sondeo, err := r.verificar(ctx, c, cand.Base)
		if err != nil {
			fallos = append(fallos, fmt.Sprintf("%s (%s): %v", anfitrion(cand.Base), cand.Origen, err))
			continue
		}

		c.Mudar(cand.Base)
		r.apuntar(c.Nombre(), Comprobacion{
			Base: cand.Base, Cuando: time.Now(), Origen: cand.Origen, Candidatos: len(cs),
		})
		r.recordar(c.Nombre(), cand.Base)

		// Este log es la señal de que algo se ha movido ahí fuera. Va a INFO
		// aunque sea raro precisamente porque es raro y hay que poder verlo.
		r.log.Info("el sitio se ha mudado y se ha adoptado el dominio nuevo",
			"sitio", c.Nombre(),
			"antes", anfitrion(actual),
			"ahora", anfitrion(cand.Base),
			"encontrado_por", cand.Origen,
			"resultados_de_prueba", sondeo.Resultados)
		return nil
	}

	err := fmt.Errorf("ningún candidato pasó la verificación (%d probados)", len(cs))
	r.apuntar(c.Nombre(), Comprobacion{
		Base: actual, Cuando: time.Now(), Error: err.Error(), Candidatos: len(cs),
	})
	if len(fallos) > 0 {
		r.log.Warn("candidatos descartados",
			"sitio", c.Nombre(), "motivos", strings.Join(fallos, "; "))
	}
	return err
}

// errores de verificación, para poder distinguirlos en los tests.
var (
	ErrMarcaFalta  = errors.New("no tiene las marcas del sitio")
	ErrParking     = errors.New("parece un parking de dominios")
	ErrSinAciertos = errors.New("la búsqueda de prueba no devolvió bastantes resultados")
)

// verificar decide si un candidato es de verdad el sitio.
//
// Las tres pruebas del plan, y ninguna sobra: las marcas positivas descartan al
// que no se le parece, las negativas al parking que sí se le parece, y la
// búsqueda de prueba al que ha copiado el diseño entero. Un parking sabe
// clonar una página; no sabe buscar "matrix".
func (r *Resolutor) verificar(ctx context.Context, c conectores.Mudable, base string) (conectores.Sondeo, error) {
	if normalizarBase(base) == "" {
		return conectores.Sondeo{}, fmt.Errorf("dirección inválida")
	}

	sondeo, err := c.Sondear(ctx, base)
	if err != nil {
		return sondeo, err
	}
	h := c.Huella()
	html := strings.ToLower(sondeo.HTML)

	for _, marca := range h.Contiene {
		if !strings.Contains(html, strings.ToLower(marca)) {
			return sondeo, fmt.Errorf("%w: falta %q", ErrMarcaFalta, marca)
		}
	}
	for _, marca := range append(conectores.MarcasDeParking, h.NoContiene...) {
		if strings.Contains(html, strings.ToLower(marca)) {
			return sondeo, fmt.Errorf("%w: contiene %q", ErrParking, marca)
		}
	}
	if sondeo.Resultados < h.MinAciertos {
		return sondeo, fmt.Errorf("%w: %d de %d buscando %q",
			ErrSinAciertos, sondeo.Resultados, h.MinAciertos, h.Consulta)
	}
	return sondeo, nil
}

// Comprobaciones devuelve cómo fue la última revisión de cada sitio.
func (r *Resolutor) Comprobaciones() map[string]Comprobacion {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]Comprobacion, len(r.ultima))
	for k, v := range r.ultima {
		out[k] = v
	}
	return out
}

func (r *Resolutor) apuntar(nombre string, c Comprobacion) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ultima[nombre] = c
}

// recordar guarda el dominio bueno para el próximo arranque. Que no se pueda
// escribir no es motivo para dejar de funcionar: solo significa que el
// siguiente arranque volverá a descubrirlo.
func (r *Resolutor) recordar(nombre, base string) {
	if r.estado == nil {
		return
	}
	if err := r.estado.Guardar(nombre, base); err != nil {
		r.log.Warn("no se pudo guardar el estado", "sitio", nombre, "error", err)
	}
}

func (r *Resolutor) mudables() []conectores.Mudable {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]conectores.Mudable, 0, len(r.sitios))
	for _, c := range r.sitios {
		out = append(out, c)
	}
	return out
}
