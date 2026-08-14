package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/davic80/iman"
	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/dominios"
	"github.com/davic80/iman/internal/novedades"
	"github.com/davic80/iman/internal/tmdb"
)

// EstadoConector es lo que /salud muestra de cada sitio.
//
// Es una vista, no el dato: el buscador lleva la cuenta de verdad y aquí solo
// se traduce a algo que se pueda pintar. Existe porque los sitios que se
// scrapean cambian de dominio y de maquetado sin avisar, y hace falta un sitio
// donde mirar qué se ha roto y cuándo.
type EstadoConector struct {
	Nombre     string
	Estado     string // "vivo", "degradado", "caido", "sin datos"
	Dominio    string
	Comprobado string
	Latencia   string
	Errores    int

	// Lo que sabe el resolutor de dominios: cuándo se verificó por última vez
	// que ese dominio es de verdad el sitio, y por qué camino se encontró.
	// Vacíos mientras el vigilante no haya hecho su primera ronda.
	Verificado string
	Origen     string
	Aviso      string
}

// Clase traduce el estado a un modificador CSS.
func (e EstadoConector) Clase() string {
	switch e.Estado {
	case buscador.Vivo:
		return "bien"
	// Un sitio al que todavía no se le ha preguntado nada no está bien ni mal:
	// no se sabe, y pintarlo de verde sería inventárselo.
	case buscador.Degradado, buscador.SinDatos:
		return "regular"
	default:
		return "mal"
	}
}

type Servidor struct {
	cfg        Config
	log        *slog.Logger
	plantillas juegoPlantillas
	arranque   time.Time
	motor      *buscador.Buscador
	novedades  *novedades.Rondin
	vigilante  Vigilante
	carteles   *tmdb.Cliente
}

// Vigilante es lo único que /salud necesita del resolutor de dominios: cómo fue
// la última comprobación de cada sitio.
type Vigilante interface {
	Comprobaciones() map[string]dominios.Comprobacion
}

// ConVigilante engancha el resolutor de dominios, si lo hay. Es opcional: sin
// él /salud enseña el dominio en uso pero no desde cuándo se sabe que es bueno.
func (s *Servidor) ConVigilante(v Vigilante) { s.vigilante = v }

// ConNovedades engancha el rondín que mantiene la portada. También es opcional:
// sin él /novedades existe pero está vacía, que es lo que ven los tests y lo que
// se ve en el primer arranque hasta que acabe la primera ronda.
func (s *Servidor) ConNovedades(r *novedades.Rondin) {
	if r != nil {
		s.novedades = r
	}
}

// ConTMDB engancha el enriquecedor de fichas. Opcional también, y de verdad:
// sin clave de TMDB Imán se comporta exactamente como antes de que existiera,
// solo que sin carátulas.
func (s *Servidor) ConTMDB(c *tmdb.Cliente) { s.carteles = c }

// Nuevo monta el servidor. Con motor nil arranca sin sitios que consultar, que
// es lo que hacen los tests que solo miran el HTML.
func Nuevo(cfg Config, log *slog.Logger, motor *buscador.Buscador) (*Servidor, error) {
	plantillas, err := compilarPlantillas(iman.Plantillas)
	if err != nil {
		return nil, err
	}
	if motor == nil {
		motor = buscador.Nuevo(log, cfg.TiempoBusqueda)
	}
	return &Servidor{
		cfg:        cfg,
		log:        log,
		plantillas: plantillas,
		arranque:   time.Now(),
		motor:      motor,
		// Un rondín sin sitios: la portada sale vacía en vez de tener que
		// comprobar en cada handler si hay alguien detrás.
		novedades: novedades.Nuevo(log, nil),
	}, nil
}

func (s *Servidor) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.paginaBuscar)
	mux.HandleFunc("GET /buscar", s.paginaBuscar)
	mux.HandleFunc("GET /novedades", s.paginaNovedades)
	mux.HandleFunc("POST /novedades/refrescar", s.refrescarNovedades)
	mux.HandleFunc("GET /salud", s.paginaSalud)

	// El magnet y el .torrent los pide el servidor, no el navegador: quien
	// busca no tiene por qué acabar conectándose al sitio de torrents. Las
	// carátulas van por lo mismo.
	mux.HandleFunc("GET /magnet", s.magnet)
	mux.HandleFunc("GET /torrent", s.torrent)
	mux.HandleFunc("GET /cartel/{tamaño}/{fichero}", s.cartel)

	// Sonda para el HEALTHCHECK del contenedor: texto plano, sin plantillas,
	// sin tocar nada externo. Solo responde si el proceso esta en pie.
	mux.HandleFunc("GET /vivo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})

	mux.Handle("GET /estaticos/", ficherosEstaticos())

	return registrar(s.log, mux)
}

type datosBase struct {
	Version string

	// TMDB dice si hay carátulas. Lo mira el pie para dar el crédito, que es lo
	// que TMDB pide a cambio de su API y no se pone cuando no se está usando.
	TMDB bool
}

// base son los datos que toda página necesita.
func (s *Servidor) base() datosBase {
	return datosBase{Version: s.cfg.Version, TMDB: s.carteles.Activo()}
}

type datosSalud struct {
	datosBase
	Arranque   string
	EnPie      string
	Busquedas  int
	Conectores []EstadoConector
}

func (s *Servidor) paginaSalud(w http.ResponseWriter, r *http.Request) {
	s.pintar(w, r, "salud", datosSalud{
		datosBase:  s.base(),
		Arranque:   s.arranque.Format("2006-01-02 15:04:05 MST"),
		EnPie:      time.Since(s.arranque).Truncate(time.Second).String(),
		Busquedas:  s.motor.Busquedas(),
		Conectores: s.estado(),
	})
}

// estado traduce la salud que lleva el buscador a algo pintable, y le añade lo
// que sepa el resolutor sobre el dominio.
func (s *Servidor) estado() []EstadoConector {
	var verificaciones map[string]dominios.Comprobacion
	if s.vigilante != nil {
		verificaciones = s.vigilante.Comprobaciones()
	}

	salud := s.motor.Salud()
	out := make([]EstadoConector, 0, len(salud))
	for _, x := range salud {
		e := EstadoConector{
			Nombre:  x.Nombre,
			Estado:  x.Estado(),
			Dominio: x.Dominio,
			Errores: x.Fallos,
		}
		if !x.UltimaVez.IsZero() {
			e.Comprobado = x.UltimaVez.Format("15:04:05")
			e.Latencia = x.Latencia.Round(time.Millisecond).String()
		}
		if v, ok := verificaciones[x.Nombre]; ok {
			e.Verificado = v.Cuando.Format("15:04:05")
			e.Origen = v.Origen
			if v.Error != "" {
				// Que el resolutor no encuentre dominio bueno es lo más grave
				// que puede pasarle a un sitio: no es que vaya lento, es que no
				// se sabe dónde está.
				e.Aviso = v.Error
			}
		}
		out = append(out, e)
	}
	return out
}

// registrar deja una linea por peticion con su codigo y su duracion.
func registrar(log *slog.Logger, siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		env := &envoltorio{ResponseWriter: w, codigo: http.StatusOK}
		siguiente.ServeHTTP(env, r)
		log.Info("peticion",
			"metodo", r.Method,
			"ruta", r.URL.Path,
			"codigo", env.codigo,
			"duracion", time.Since(inicio).Round(time.Millisecond),
		)
	})
}

// envoltorio captura el codigo de estado, que el ResponseWriter no expone.
type envoltorio struct {
	http.ResponseWriter
	codigo int
}

func (e *envoltorio) WriteHeader(codigo int) {
	e.codigo = codigo
	e.ResponseWriter.WriteHeader(codigo)
}
