// Package tmdb le pone cara a lo que encuentran los sitios.
//
// Los sitios de torrents publican una cadena de texto y poco más: ni carátula,
// ni sinopsis, ni el título oficial. TMDB tiene todo eso, así que Imán le
// pregunta por la obra que ha deducido del título y se queda con la ficha.
//
// La regla de la casa aquí es que **una carátula equivocada es peor que
// ninguna**: se pide fiable antes que vistoso. Si el título que devuelve TMDB
// no es exactamente la obra que se buscó, esta fila se queda sin cara.
//
// Nada de esto es imprescindible: sin clave, o con TMDB caído, Imán funciona
// igual que antes. Es decoración, y se comporta como tal.
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/davic80/iman/internal/titulos"
)

// APIPorDefecto e ImagenesPorDefecto son los dos servidores de TMDB: uno
// contesta JSON y el otro sirve las imágenes. Se pueden cambiar para los tests.
const (
	APIPorDefecto      = "https://api.themoviedb.org/3"
	ImagenesPorDefecto = "https://image.tmdb.org/t/p"
)

// TamañoCartel es el ancho que se pide de la carátula. En la lista se enseña
// pequeña, así que pedir el original sería tirar megas a la basura.
const TamañoCartel = "w185"

// maxCache es cuántas fichas se recuerdan a la vez. Es un tope de memoria, no
// una caducidad: una película no cambia de cartel, y el proceso se reinicia en
// cada despliegue.
const maxCache = 5000

// maxCuerpo es cuánto se lee como mucho de una respuesta de la API. Una página
// de resultados de TMDB no llega a 100 KB.
const maxCuerpo = 1 << 20

// Ficha es lo que TMDB sabe de una obra.
type Ficha struct {
	ID       int
	Titulo   string // El oficial en castellano, que suele ser mejor que el del sitio
	Año      int
	Cartel   string // Ruta dentro de TMDB, tipo "/abc.jpg". Vacía si no tiene
	Sinopsis string
}

// Cliente habla con TMDB y recuerda lo que ya ha preguntado.
//
// El cero vale y no hace nada: un Cliente sin clave contesta "no hay ficha" a
// todo, que es exactamente lo que tiene que pasar cuando nadie ha configurado
// TMDB. Un puntero nil también, para que quien lo use no tenga que comprobarlo.
type Cliente struct {
	token    string
	v3       bool // La clave vieja va en la URL; el token nuevo, en la cabecera
	api      string
	imagenes string
	http     *http.Client
	log      *slog.Logger

	mu    sync.Mutex
	cache map[string]*entrada
}

// entrada es una consulta a TMDB, hecha o en marcha.
//
// El canal es lo que evita que doce filas de la misma película pregunten doce
// veces: la primera abre la entrada y las demás esperan a que la cierre. Que
// pasa siempre, porque en la portada hay muchas repeticiones de lo mismo.
type entrada struct {
	listo chan struct{}
	ficha Ficha
	hay   bool
}

// Nuevo crea el cliente. Sin token devuelve uno apagado: no es un error, es la
// configuración por defecto de quien no ha pedido clave a TMDB.
func Nuevo(token string, log *slog.Logger) *Cliente {
	if log == nil {
		log = slog.Default()
	}
	return &Cliente{
		token:    strings.TrimSpace(token),
		v3:       esClaveV3(token),
		api:      APIPorDefecto,
		imagenes: ImagenesPorDefecto,
		http: &http.Client{
			// TMDB es rápido y esto va dentro del tiempo que espera el usuario:
			// si tarda más que esto, la carátula no merece seguir esperando.
			Timeout: 5 * time.Second,
		},
		log:   log,
		cache: map[string]*entrada{},
	}
}

// Activo dice si hay clave con la que preguntar.
func (c *Cliente) Activo() bool { return c != nil && c.token != "" }

// Contra apunta el cliente a otros servidores. Es para los tests: en producción
// los de TMDB son siempre los mismos y no hay nada que configurar.
func (c *Cliente) Contra(api, imagenes string) *Cliente {
	c.api, c.imagenes = api, imagenes
	return c
}

// reClaveV3 reconoce la clave de siempre de TMDB, que son 32 caracteres
// hexadecimales. El token nuevo es un JWT, mucho más largo y con puntos.
var reClaveV3 = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// esClaveV3 distingue las dos credenciales que da TMDB en la misma página. Se
// aceptan las dos porque son fáciles de confundir y el error que produce
// equivocarse —un 401 -- no explica cuál de las dos hacía falta.
func esClaveV3(token string) bool { return reClaveV3.MatchString(strings.TrimSpace(token)) }

// Buscar devuelve la ficha de una obra, si TMDB la tiene y es sin duda la
// misma. Nunca devuelve error: quedarse sin carátula no es un fallo de la
// página, así que lo que sale mal se apunta en el log y aquí se dice que no.
// El título entero del sitio va aparte de la obra porque el clasificador corta
// por marcas de release y a veces se lleva parte del nombre: "Una Milla:
// Capítulo Uno" queda en "una milla", que es justo como TMDB no la llama.
func (c *Cliente) Buscar(ctx context.Context, titulo string, info titulos.Info) (Ficha, bool) {
	if !c.Activo() || info.Obra == "" {
		return Ficha{}, false
	}

	clave := fmt.Sprintf("%t|%s|%d", info.EsSerie(), info.Obra, info.Año)

	c.mu.Lock()
	if e, ya := c.cache[clave]; ya {
		c.mu.Unlock()
		select {
		case <-e.listo:
			return e.ficha, e.hay
		case <-ctx.Done():
			return Ficha{}, false
		}
	}
	// Cuando se llena se tira entera. Es tosco, pero una caché de carátulas no
	// merece un LRU: lo peor que pasa es preguntar otra vez.
	if len(c.cache) >= maxCache {
		c.cache = map[string]*entrada{}
	}
	e := &entrada{listo: make(chan struct{})}
	c.cache[clave] = e
	c.mu.Unlock()

	candidatos, err := c.consultar(ctx, info.Obra, info.EsSerie())
	if err == nil {
		buscado := nombres(info.Obra, titulo)
		if elegido, vale := elegir(buscado, info.Año, info.EsSerie(), candidatos); vale {
			e.ficha, e.hay = elegido.ficha(), true
		}
	}
	close(e.listo)

	if err != nil {
		// Un fallo de red no es "esta película no está en TMDB": se borra la
		// entrada para que la siguiente vez se vuelva a intentar.
		c.mu.Lock()
		delete(c.cache, clave)
		c.mu.Unlock()

		if ctx.Err() == nil {
			c.log.Warn("TMDB no contestó", "obra", info.Obra, "error", err)
		}
		return Ficha{}, false
	}
	return e.ficha, e.hay
}

// candidato es un resultado de TMDB ya traducido a lo que aquí importa.
type candidato struct {
	ID       int
	Titulo   string // El castellano
	Original string
	Año      int
	Cartel   string
	Sinopsis string
}

func (c candidato) ficha() Ficha {
	return Ficha{ID: c.ID, Titulo: c.Titulo, Año: c.Año, Cartel: c.Cartel, Sinopsis: c.Sinopsis}
}

// crudo es un resultado tal cual lo escribe TMDB. Las películas y las series
// usan nombres de campo distintos para lo mismo, así que caben los dos juegos.
type crudo struct {
	ID             int    `json:"id"`
	Titulo         string `json:"title"`
	TituloOriginal string `json:"original_title"`
	Nombre         string `json:"name"`
	NombreOriginal string `json:"original_name"`
	Estreno        string `json:"release_date"`
	Emision        string `json:"first_air_date"`
	Cartel         string `json:"poster_path"`
	Sinopsis       string `json:"overview"`
}

func (c crudo) candidato() candidato {
	titulo, original, fecha := c.Titulo, c.TituloOriginal, c.Estreno
	if titulo == "" {
		titulo, original, fecha = c.Nombre, c.NombreOriginal, c.Emision
	}
	año := 0
	if len(fecha) >= 4 {
		año, _ = strconv.Atoi(fecha[:4])
	}
	return candidato{
		ID: c.ID, Titulo: titulo, Original: original, Año: año,
		Cartel: c.Cartel, Sinopsis: c.Sinopsis,
	}
}

// consultar le pregunta a TMDB por una obra. Se pide en castellano porque el
// título que se enseña tiene que ser el que el usuario reconoce.
func (c *Cliente) consultar(ctx context.Context, obra string, serie bool) ([]candidato, error) {
	ruta := "/search/movie"
	if serie {
		ruta = "/search/tv"
	}

	q := url.Values{
		"query":         {obra},
		"language":      {"es-ES"},
		"include_adult": {"false"},
	}
	if c.v3 {
		q.Set("api_key", c.token)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.api+ruta+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if !c.v3 {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("TMDB no acepta la clave (401): revisa IMAN_TMDB")
		}
		return nil, fmt.Errorf("TMDB devolvió %d", resp.StatusCode)
	}

	var cuerpo struct {
		Resultados []crudo `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxCuerpo)).Decode(&cuerpo); err != nil {
		return nil, fmt.Errorf("respuesta ilegible de TMDB: %w", err)
	}

	cs := make([]candidato, 0, len(cuerpo.Resultados))
	for _, r := range cuerpo.Resultados {
		cs = append(cs, r.candidato())
	}
	return cs, nil
}

// elegir se queda con el candidato que es la obra buscada, o con ninguno.
//
// TMDB ordena por relevancia, pero su relevancia es la de un buscador: pedir
// "el padrino" devuelve también documentales sobre El Padrino. Aquí solo pasa
// lo que se llama igual, y el año decide entre los homónimos.
func elegir(buscado []string, año int, serie bool, cs []candidato) (candidato, bool) {
	var posibles []candidato
	for _, c := range cs {
		if !mismoTitulo(buscado, c) {
			continue
		}
		// El año veta en las películas: dos películas del mismo título son dos
		// películas distintas, casi siempre un remake. En las series no, porque
		// el año que trae el título de un capítulo es el del capítulo, no el del
		// estreno de la serie.
		if !serie && año > 0 && c.Año > 0 && distancia(c.Año, año) > 1 {
			continue
		}
		posibles = append(posibles, c)
	}
	if len(posibles) == 0 {
		return candidato{}, false
	}

	// Entre los que quedan gana el del año exacto; si no lo hay, el que TMDB
	// puso primero, que es el más conocido.
	if año > 0 {
		for _, c := range posibles {
			if c.Año == año {
				return c, true
			}
		}
	}
	return posibles[0], true
}

// mismoTitulo mira si alguna forma de escribir lo que se busca coincide con
// alguna de escribir el candidato. Sigue siendo igualdad: lo que se amplía es
// cómo se escribe lo mismo, no cuánto se parece.
func mismoTitulo(buscado []string, c candidato) bool {
	suyos := nombres(c.Titulo, c.Original)
	for _, b := range buscado {
		for _, s := range suyos {
			if b == s {
				return true
			}
		}
	}
	return false
}

// reParentesisFinal quita la coletilla entre paréntesis con la que TMDB
// desempata títulos repetidos: "Incontrolable (I Swear)" es la misma película
// que el sitio publica como "Incontrolable".
var reParentesisFinal = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

// nombres devuelve las formas en que se puede escribir un título: tal cual, sin
// el artículo inicial y sin el paréntesis final. Los sitios escriben "Matrix"
// donde TMDB dice "The Matrix", y al revés.
func nombres(ts ...string) []string {
	var out []string
	for _, t := range ts {
		for _, v := range []string{t, reParentesisFinal.ReplaceAllString(t, "")} {
			n := titulos.Normalizar(v)
			if n == "" {
				continue
			}
			out = append(out, n)
			if corto := titulos.SinArticulo(n); corto != n {
				out = append(out, corto)
			}
		}
	}
	return out
}

func distancia(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// reCartel comprueba que una ruta de carátula es lo que dice ser antes de
// pedirla. Llega desde la petición del usuario, y sin esto Imán sería un proxy
// abierto contra cualquier URL que le pongan. Es la misma precaución que se
// toma con los .torrent.
// El nombre es base64 de los que llevan guiones ("/9O1Iy9od7-GeQ_xHYuoM3Pt.jpg"),
// así que entran el guión y el subrayado. La barra y el punto no, que son las
// dos formas de salirse de ahí.
var reCartel = regexp.MustCompile(`^/[A-Za-z0-9_-]{8,64}\.(jpg|png)$`)

// Cartel trae la imagen desde TMDB para que la sirva Imán.
//
// La pide el servidor y no el navegador a propósito, por lo mismo que el
// .torrent: quien mira la portada no tiene por qué aparecer en los registros de
// nadie más, y una página privada que va cargando imágenes de fuera deja de
// serlo un poco.
func (c *Cliente) Cartel(ctx context.Context, ruta string) (io.ReadCloser, string, error) {
	if !c.Activo() {
		return nil, "", fmt.Errorf("TMDB no está configurado")
	}
	if !reCartel.MatchString(ruta) {
		return nil, "", fmt.Errorf("esa no es una ruta de carátula")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.imagenes+"/"+TamañoCartel+ruta, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("TMDB devolvió %d al pedir la carátula", resp.StatusCode)
	}

	tipo := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(tipo, "image/") {
		resp.Body.Close()
		return nil, "", fmt.Errorf("eso no es una imagen (%s)", tipo)
	}
	return resp.Body, tipo, nil
}
