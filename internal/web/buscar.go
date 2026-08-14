package web

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
	"github.com/davic80/iman/internal/tmdb"
)

type datosBuscar struct {
	datosBase
	Consulta      string
	Buscado       bool // Si hay algo que enseñar debajo del formulario
	Dudosos       bool
	Filtros       barraFiltros
	Resultados    []vistaResultado
	Descartados   int
	Fallos        []buscador.Fallo
	Duracion      string
	SinConectores bool
}

// vistaResultado es un resultado listo para pintar: todo son cadenas, para que
// la plantilla no tenga que decidir nada.
type vistaResultado struct {
	Titulo     string
	Sitio      string
	Otros      []vistaOtroSitio // Los demás sitios que tienen lo mismo
	Idioma     string
	Dudoso     bool
	Calidad    string
	Tamaño     string
	Episodio   string
	Año        string
	Ficha      string
	URLMagnet  string
	URLTorrent string

	// Lo que ha puesto TMDB, si ha reconocido la película. Vacío es lo normal:
	// sin clave, sin acierto seguro o sin tiempo, la fila se pinta igual.
	Cartel       string // URL de la carátula, servida por Imán
	CartelGrande string // La misma, en grande, para cuando se pincha
	Obra         string // El título oficial en castellano, para el alt de la imagen
	Sinopsis     string
	Generos      string // Ya juntos: "Ciencia ficción · Acción"
	Nota         string // Sobre 10 y con coma: "8,3". Vacío si TMDB no tiene votos
}

// vistaOtroSitio es un sitio que publica el mismo torrent que la fila. Va con
// su enlace propio: la fusión es una apuesta, así que si Imán se ha equivocado
// juntándolos, desde aquí se llega igual a lo que publica cada uno.
type vistaOtroSitio struct {
	Sitio      string
	URLTorrent string
}

func (s *Servidor) paginaBuscar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	consulta := strings.TrimSpace(q.Get("q"))
	dudosos := q.Get("dudosos") != ""

	datos := datosBuscar{
		datosBase:     s.base(),
		Consulta:      consulta,
		Dudosos:       dudosos,
		SinConectores: s.motor.Conectores() == 0,
	}

	if consulta != "" && !datos.SinConectores {
		b := s.motor.Buscar(r.Context(), consulta, buscador.Opciones{Dudosos: dudosos})

		f := filtrosDe(q)
		datos.Buscado = true
		datos.Descartados = b.Descartados
		datos.Fallos = b.Fallos
		datos.Duracion = b.Duracion.Round(time.Millisecond).String()
		datos.Filtros = f.barra(b.Resultados, "/", q)

		// Las fichas se piden solo de lo que se va a enseñar: filtrar primero y
		// preguntar después ahorra las consultas de las filas que nadie va a ver.
		filtrados := f.aplicar(b.Resultados)
		fichas := s.fichas(r.Context(), filtrados)

		datos.Resultados = make([]vistaResultado, 0, len(filtrados))
		for i, res := range filtrados {
			datos.Resultados = append(datos.Resultados, vista(res, fichas[i]))
		}
	}

	s.pintar(w, r, "buscar", datos)
}

func vista(r buscador.Resultado, f tmdb.Ficha) vistaResultado {
	v := vistaResultado{
		Titulo:     r.Titulo,
		Sitio:      r.Sitio,
		Idioma:     r.Info.Idioma.String(),
		Dudoso:     r.Info.Idioma.Veredicto() == titulos.Duda,
		Calidad:    string(r.Info.Calidad),
		Tamaño:     conectores.FormatearTamaño(r.Tamaño),
		Ficha:      r.Ficha,
		URLMagnet:  enlaceA("/magnet", r.Resultado),
		URLTorrent: enlaceA("/torrent", r.Resultado),
	}
	for _, o := range r.Repetidos {
		v.Otros = append(v.Otros, vistaOtroSitio{
			Sitio:      o.Sitio,
			URLTorrent: enlaceA("/torrent", o),
		})
	}
	if r.Info.Año > 0 {
		v.Año = strconv.Itoa(r.Info.Año)
	}
	if r.Info.EsSerie() {
		v.Episodio = fmt.Sprintf("T%d", r.Info.Temporada)
		if r.Info.Episodio > 0 {
			v.Episodio += fmt.Sprintf("E%02d", r.Info.Episodio)
		}
	}
	// El título que se enseña sigue siendo el del sitio: es el que identifica a
	// esta copia y no a la película. De TMDB se coge la cara, no el nombre.
	if f.Cartel != "" {
		v.Cartel = "/cartel/" + tmdb.TamañoCartel + f.Cartel
		v.CartelGrande = "/cartel/" + tmdb.TamañoGrande + f.Cartel
		v.Obra = f.Titulo
		v.Sinopsis = f.Sinopsis
	}
	// El género y la nota no dependen de que haya carátula: si TMDB ha
	// reconocido la obra, lo que sabe de ella vale igual.
	v.Generos = generos(f.Generos)
	v.Nota = nota(f.Nota)
	return v
}

// maxGeneros son los géneros que caben en la fila sin comerse la línea de
// etiquetas. TMDB suele dar dos o tres, y el tercero casi nunca añade nada.
const maxGeneros = 2

func generos(gs []string) string {
	if len(gs) > maxGeneros {
		gs = gs[:maxGeneros]
	}
	return strings.Join(gs, " · ")
}

// nota deja la puntuación en una cifra decimal y con coma, que es como se
// escriben los decimales en castellano.
func nota(n float64) string {
	if n <= 0 {
		return ""
	}
	return strings.Replace(strconv.FormatFloat(n, 'f', 1, 64), ".", ",", 1)
}

// enlaceA arma la URL de descarga. El navegador solo manda el sitio y la ficha;
// el enlace real lo averigua el servidor preguntándole al conector, que es
// quien sabe qué URLs son suyas.
func enlaceA(ruta string, r conectores.Resultado) string {
	v := url.Values{"sitio": {r.Sitio}, "ficha": {r.Ficha}}
	return ruta + "?" + v.Encode()
}

// magnet manda al usuario al magnet, o se lo devuelve en texto si lo que quiere
// es copiarlo al portapapeles.
func (s *Servidor) magnet(w http.ResponseWriter, r *http.Request) {
	sitio, ficha := r.URL.Query().Get("sitio"), r.URL.Query().Get("ficha")
	if sitio == "" || ficha == "" {
		http.Error(w, "falta el sitio o la ficha", http.StatusBadRequest)
		return
	}

	res, err := s.motor.Resolver(r.Context(), sitio, ficha)
	if err != nil {
		s.log.Warn("no se pudo resolver la ficha", "sitio", sitio, "ficha", ficha, "error", err)
		http.Error(w, "no se pudo abrir esa ficha", http.StatusBadGateway)
		return
	}
	if res.Magnet == "" {
		http.Error(w, "esa ficha no tiene magnet", http.StatusNotFound)
		return
	}

	// El botón de copiar lo pide en texto: no puede seguir un redirect a
	// magnet: porque el navegador se lo daría al cliente de torrents.
	if r.URL.Query().Get("formato") == "texto" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(w, res.Magnet)
		return
	}

	http.Redirect(w, r, res.Magnet, http.StatusFound)
}

// torrent trae el fichero y se lo pasa al usuario.
//
// Lo descarga el servidor a propósito: así quien busca no se conecta al sitio
// de torrents ni aparece en sus registros.
func (s *Servidor) torrent(w http.ResponseWriter, r *http.Request) {
	sitio, ficha := r.URL.Query().Get("sitio"), r.URL.Query().Get("ficha")
	if sitio == "" || ficha == "" {
		http.Error(w, "falta el sitio o la ficha", http.StatusBadRequest)
		return
	}

	cuerpo, nombre, err := s.motor.Torrent(r.Context(), sitio, ficha)
	if err != nil {
		s.log.Warn("no se pudo traer el .torrent", "sitio", sitio, "ficha", ficha, "error", err)
		http.Error(w, "no se pudo descargar ese .torrent", http.StatusBadGateway)
		return
	}
	defer cuerpo.Close()

	w.Header().Set("Content-Type", "application/x-bittorrent")
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(nombre))
	if _, err := io.Copy(w, cuerpo); err != nil {
		// La cabecera ya salió, así que no hay forma de devolver un error
		// limpio: lo único que queda es dejar constancia.
		s.log.Warn("se cortó la descarga del .torrent", "sitio", sitio, "error", err)
	}
}
