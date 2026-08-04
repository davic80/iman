package conectores

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/davic80/iman/internal/titulos"
)

// EliteTorrentBase es el dominio que estaba vivo la última vez que se miró.
//
// Estos sitios cambian de dominio cada pocas semanas, así que esto es solo el
// punto de partida: el resolutor de dominios lo sustituye cuando descubre el
// que funciona hoy.
const EliteTorrentBase = "https://www.elitetorrent.wf"

// EliteTorrent es un WordPress con plantilla propia. Busca con ?s=, no tiene
// Cloudflare delante y etiqueta el idioma de cada resultado explícitamente,
// que es justo lo que hace falta aquí.
//
// Lo que no da: ni semillas ni clientes. Los pinta con JavaScript a partir de
// una llamada aparte, así que en el HTML vienen vacíos.
type EliteTorrent struct {
	Cliente *Cliente

	// La base se lee en cada búsqueda y la reescribe el resolutor cuando el
	// sitio se muda, así que va bajo candado: las dos cosas pasan a la vez.
	mu   sync.RWMutex
	base string
}

// NuevoEliteTorrent crea el conector. Con cliente nil usa uno propio que
// espacia las peticiones: este sitio da timeouts si se le pide deprisa.
func NuevoEliteTorrent(c *Cliente) *EliteTorrent {
	if c == nil {
		c = NuevoCliente(2 * time.Second)
	}
	return &EliteTorrent{Cliente: c, base: EliteTorrentBase}
}

// Si esto deja de compilar es que el conector ya no sabe mudarse y el resolutor
// lo dejaría atrás en silencio.
var _ Mudable = (*EliteTorrent)(nil)

func (e *EliteTorrent) Nombre() string { return "EliteTorrent" }

// Dominio es dónde se está buscando hoy. Lo enseña /salud, porque cuando este
// sitio deja de devolver nada suele ser que se ha mudado.
func (e *EliteTorrent) Dominio() string {
	u, err := url.Parse(e.Base())
	if err != nil {
		return ""
	}
	return u.Host
}

// Base es la URL en la que se busca ahora mismo.
func (e *EliteTorrent) Base() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.base == "" {
		return EliteTorrentBase
	}
	return strings.TrimSuffix(e.base, "/")
}

// Mudar cambia el dominio. Lo llama el resolutor cuando ha verificado que el
// sitio se ha movido; el conector se fía porque no es quien para juzgarlo.
func (e *EliteTorrent) Mudar(base string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.base = strings.TrimSuffix(base, "/")
}

// Semillas son los dominios por los que empezar a buscar el sitio. El orden es
// el de probabilidad: primero el que funcionaba, y detrás los anteriores, que
// siguen sirviendo para seguir la redirección al nuevo.
func (e *EliteTorrent) Semillas() []string {
	return []string{
		EliteTorrentBase,
		"https://www.elitetorrent.ws",
		"https://www.elitetorrent.pl",
		"https://elitetorrent.si",
	}
}

// Huella es cómo se reconoce a EliteTorrent. Las marcas positivas son de la
// plantilla, no del contenido: los títulos de las películas cambian cada día,
// pero la rejilla de resultados y el pie llevan años igual.
func (e *EliteTorrent) Huella() Huella {
	return Huella{
		Contiene:   []string{"elitetorrent", "miniboxs-ficha"},
		NoContiene: []string{"elitetorrent.click"},
		// "matrix" es la búsqueda de prueba porque tiene varias entregas: un
		// título con una sola copia podría desaparecer del sitio y dejarnos
		// rechazando el dominio bueno.
		Consulta:    "matrix",
		MinAciertos: 3,
	}
}

// Sondear busca contra una base concreta sin adoptarla. Devuelve el HTML tal
// cual, para poder mirarle las marcas, y cuántos resultados entendió el parser.
func (e *EliteTorrent) Sondear(ctx context.Context, base string) (Sondeo, error) {
	sonda := &EliteTorrent{Cliente: e.Cliente, base: base}

	doc, err := e.Cliente.Documento(ctx, sonda.URLBusqueda(e.Huella().Consulta))
	if err != nil {
		return Sondeo{}, err
	}
	html, err := doc.Html()
	if err != nil {
		return Sondeo{}, fmt.Errorf("no se pudo leer el HTML: %w", err)
	}
	// Un parser que revienta con el HTML de un candidato es motivo de sobra
	// para no adoptarlo, pero no para tumbar al vigilante.
	rs, err := sonda.parsearBusqueda(doc)
	if err != nil {
		return Sondeo{HTML: html}, err
	}
	return Sondeo{HTML: html, Resultados: len(rs)}, nil
}

// URLBusqueda arma la URL de búsqueda. Es un WordPress de toda la vida: la
// consulta va en ?s= y no hay más parámetros que valgan.
func (e *EliteTorrent) URLBusqueda(consulta string) string {
	v := url.Values{"s": {consulta}}
	return e.Base() + "/?" + v.Encode()
}

func (e *EliteTorrent) Buscar(ctx context.Context, consulta string) ([]Resultado, error) {
	doc, err := e.Cliente.Documento(ctx, e.URLBusqueda(consulta))
	if err != nil {
		return nil, fmt.Errorf("elitetorrent: %w", err)
	}
	return e.parsearBusqueda(doc)
}

// parsearBusqueda saca los resultados de la página. Va aparte de Buscar para
// poder probarla contra un HTML guardado, sin tocar la red.
func (e *EliteTorrent) parsearBusqueda(doc *goquery.Document) ([]Resultado, error) {
	base, err := url.Parse(e.Base())
	if err != nil {
		return nil, fmt.Errorf("elitetorrent: base inválida: %w", err)
	}

	var out []Resultado
	doc.Find("ul.miniboxs-ficha li").Each(func(_ int, li *goquery.Selection) {
		enlace := li.Find("div.meta a.nombre")
		titulo := strings.TrimSpace(enlace.Text())
		href, _ := enlace.Attr("href")
		if titulo == "" || href == "" {
			// Sin título ni enlace no hay nada que ofrecer. Se salta en vez de
			// devolver error: una fila rota no debe tirar la búsqueda entera.
			return
		}

		r := Resultado{
			Sitio:    e.Nombre(),
			Titulo:   titulo,
			Ficha:    absoluta(base, href),
			Tamaño:   ParsearTamaño(li.Find("span.dig1").First().Text()),
			Semillas: -1, // El sitio no las publica
			Clientes: -1,
			Info:     titulos.Analizar(titulo),
		}

		// El sitio declara el idioma en el title de la banderita. Cuando lo
		// dice él vale más que lo que se pueda deducir del título.
		if marca, ok := li.Find("span#idiomacio img").First().Attr("title"); ok {
			if idioma := titulos.DetectarIdioma(marca); idioma != titulos.Desconocido {
				r.Info.Idioma = idioma
			}
		}

		// La calidad va en el otro span.marca, el que lleva texto en vez de
		// imagen. A veces está vacío o pone "---"; entonces se mira el título,
		// que en resultados como "Matrix (HDRip)" la lleva dentro.
		if c := titulos.DetectarCalidad(etiquetaCalidad(li)); c != titulos.CalidadDesconocida {
			r.Info.Calidad = c
		}

		out = append(out, r)
	})
	return out, nil
}

// etiquetaCalidad devuelve el texto del span.marca que no es la bandera del
// idioma. Se distinguen por el contenido: el del idioma es una imagen y no
// tiene texto.
func etiquetaCalidad(li *goquery.Selection) string {
	var etiqueta string
	li.Find("span.marca i").Each(func(_ int, s *goquery.Selection) {
		if t := strings.TrimSpace(s.Text()); t != "" {
			etiqueta = t
		}
	})
	return etiqueta
}

// Resolver pide la ficha y rellena el magnet y el .torrent, que en la página
// de resultados no vienen.
func (e *EliteTorrent) Resolver(ctx context.Context, r *Resultado) error {
	if r.Ficha == "" {
		return fmt.Errorf("elitetorrent: resultado sin ficha")
	}
	if err := e.esMia(r.Ficha); err != nil {
		return err
	}
	doc, err := e.Cliente.Documento(ctx, r.Ficha)
	if err != nil {
		return fmt.Errorf("elitetorrent: %w", err)
	}
	return e.parsearFicha(doc, r)
}

func (e *EliteTorrent) parsearFicha(doc *goquery.Document, r *Resultado) error {
	base, err := url.Parse(e.Base())
	if err != nil {
		return fmt.Errorf("elitetorrent: base inválida: %w", err)
	}

	if m, ok := doc.Find(`a[href^="magnet:"]`).First().Attr("href"); ok {
		r.Magnet = m
	}
	if t, ok := doc.Find("a.enlace_torrent").First().Attr("href"); ok {
		r.Torrent = absoluta(base, t)
	}
	if r.Magnet == "" && r.Torrent == "" {
		return fmt.Errorf("elitetorrent: la ficha %s no tiene ni magnet ni .torrent", r.Ficha)
	}

	// La ficha trae una tabla de datos más fiable que las etiquetas de la
	// página de resultados, así que lo que diga aquí manda.
	for etiqueta, valor := range fichaTecnica(doc) {
		switch etiqueta {
		case "tamaño":
			if t := ParsearTamaño(valor); t > 0 {
				r.Tamaño = t
			}
		case "fecha":
			if f, err := time.Parse("2006-01-02", valor); err == nil {
				r.Fecha = f
			}
		case "idioma":
			if i := titulos.DetectarIdioma(valor); i != titulos.Desconocido {
				r.Info.Idioma = i
			}
		case "calidad":
			if c := titulos.DetectarCalidad(valor); c != titulos.CalidadDesconocida {
				r.Info.Calidad = c
			}
		}
	}
	return nil
}

// fichaTecnica lee el bloque "Información técnica", que son pares
// "<b>Etiqueta:</b> valor" sueltos dentro de un párrafo.
func fichaTecnica(doc *goquery.Document) map[string]string {
	datos := map[string]string{}
	doc.Find("p.descrip span").Each(func(_ int, s *goquery.Selection) {
		etiqueta := strings.TrimSpace(s.Find("b").First().Text())
		if etiqueta == "" {
			return
		}
		valor := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s.Text()), etiqueta))
		clave := strings.ToLower(strings.TrimSuffix(etiqueta, ":"))
		if valor != "" {
			datos[clave] = valor
		}
	})
	return datos
}

// Descargar trae el fichero .torrent.
//
// Lo pide el servidor y no el navegador del usuario, para que quien busca no
// tenga que conectarse al sitio ni aparecer en sus registros.
func (e *EliteTorrent) Descargar(ctx context.Context, torrent string) (io.ReadCloser, error) {
	if err := e.esMia(torrent); err != nil {
		return nil, err
	}
	return e.Cliente.Traer(ctx, torrent)
}

// esMia comprueba que una URL es de este sitio.
//
// Las URLs que llegan a Resolver y a Descargar vienen de la petición del
// usuario, así que sin esta comprobación cualquiera podría usar Imán para
// pedir en su nombre lo que quisiera, incluida la red interna del servidor.
func (e *EliteTorrent) esMia(dir string) error {
	base, err := url.Parse(e.Base())
	if err != nil {
		return fmt.Errorf("elitetorrent: base inválida: %w", err)
	}
	u, err := url.Parse(dir)
	if err != nil {
		return fmt.Errorf("elitetorrent: url inválida %q: %w", dir, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("elitetorrent: %q no es una url http", dir)
	}
	if !mismoSitio(u.Hostname(), base.Hostname()) {
		return fmt.Errorf("elitetorrent: %q no es de este sitio", u.Hostname())
	}
	return nil
}

// mismoSitio acepta el dominio y sus subdominios: el sitio mezcla
// "elitetorrent.wf" y "www.elitetorrent.wf" en sus propios enlaces.
func mismoSitio(anfitrion, base string) bool {
	anfitrion, base = strings.ToLower(anfitrion), strings.ToLower(base)
	base = strings.TrimPrefix(base, "www.")
	return anfitrion == base || strings.HasSuffix(anfitrion, "."+base)
}

// absoluta convierte un enlace relativo en absoluto. Estos sitios mezclan las
// dos formas dentro de la misma página.
func absoluta(base *url.URL, href string) string {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return href
	}
	return base.ResolveReference(u).String()
}
