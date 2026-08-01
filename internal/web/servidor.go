package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/davic80/iman"
)

// EstadoConector es lo que /salud muestra de cada sitio.
//
// Existe ya en la fase 0, sin conectores que la rellenen, porque es la pieza
// que hace operable el proyecto: los sitios que scrapeamos cambian de dominio y
// de maquetado sin avisar, y hace falta un sitio donde mirar que se ha roto y
// cuando.
type EstadoConector struct {
	Nombre     string
	Estado     string // "vivo", "degradado", "caido"
	Dominio    string
	Comprobado string
	Latencia   string
	Errores    int
}

// Clase traduce el estado a un modificador CSS.
func (e EstadoConector) Clase() string {
	switch e.Estado {
	case "vivo":
		return "bien"
	case "degradado":
		return "regular"
	default:
		return "mal"
	}
}

// FuenteEstado devuelve el estado de los conectores. En la fase 1 la
// implementara el buscador; de momento el servidor no sabe nada de conectores y
// eso esta bien: la web no deberia acoplarse al scraping.
type FuenteEstado func() []EstadoConector

type Servidor struct {
	cfg        Config
	log        *slog.Logger
	plantillas juegoPlantillas
	arranque   time.Time
	estado     FuenteEstado
}

func Nuevo(cfg Config, log *slog.Logger, estado FuenteEstado) (*Servidor, error) {
	plantillas, err := compilarPlantillas(iman.Plantillas)
	if err != nil {
		return nil, err
	}
	if estado == nil {
		estado = func() []EstadoConector { return nil }
	}
	return &Servidor{
		cfg:        cfg,
		log:        log,
		plantillas: plantillas,
		arranque:   time.Now(),
		estado:     estado,
	}, nil
}

func (s *Servidor) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.paginaBuscar)
	mux.HandleFunc("GET /buscar", s.paginaBuscar)
	mux.HandleFunc("GET /salud", s.paginaSalud)

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

type datosBuscar struct {
	datosBase
	Consulta      string
	SinConectores bool
}

func (s *Servidor) paginaBuscar(w http.ResponseWriter, r *http.Request) {
	s.pintar(w, r, "buscar", datosBuscar{
		datosBase:     datosBase{Version: s.cfg.Version},
		Consulta:      r.URL.Query().Get("q"),
		SinConectores: len(s.estado()) == 0,
	})
}

type datosSalud struct {
	datosBase
	Arranque   string
	EnPie      string
	Conectores []EstadoConector
}

func (s *Servidor) paginaSalud(w http.ResponseWriter, r *http.Request) {
	s.pintar(w, r, "salud", datosSalud{
		datosBase:  datosBase{Version: s.cfg.Version},
		Arranque:   s.arranque.Format("2006-01-02 15:04:05 MST"),
		EnPie:      time.Since(s.arranque).Truncate(time.Second).String(),
		Conectores: s.estado(),
	})
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
