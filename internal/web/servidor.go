package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/davic80/iman"
	"github.com/davic80/iman/internal/buscador"
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
}

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
	}, nil
}

func (s *Servidor) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.paginaBuscar)
	mux.HandleFunc("GET /buscar", s.paginaBuscar)
	mux.HandleFunc("GET /salud", s.paginaSalud)

	// El magnet y el .torrent los pide el servidor, no el navegador: quien
	// busca no tiene por qué acabar conectándose al sitio de torrents.
	mux.HandleFunc("GET /magnet", s.magnet)
	mux.HandleFunc("GET /torrent", s.torrent)

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
		datosBase:  datosBase{Version: s.cfg.Version},
		Arranque:   s.arranque.Format("2006-01-02 15:04:05 MST"),
		EnPie:      time.Since(s.arranque).Truncate(time.Second).String(),
		Busquedas:  s.motor.Busquedas(),
		Conectores: s.estado(),
	})
}

// estado traduce la salud que lleva el buscador a algo pintable.
func (s *Servidor) estado() []EstadoConector {
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
