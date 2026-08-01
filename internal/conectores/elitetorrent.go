package conectores

import (
	"context"
	"fmt"
	"net/url"
	"strings"
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
	Base    string
}

// NuevoEliteTorrent crea el conector. Con cliente nil usa uno propio que
// espacia las peticiones: este sitio da timeouts si se le pide deprisa.
func NuevoEliteTorrent(c *Cliente) *EliteTorrent {
	if c == nil {
		c = NuevoCliente(2 * time.Second)
	}
	return &EliteTorrent{Cliente: c, Base: EliteTorrentBase}
}

func (e *EliteTorrent) Nombre() string { return "EliteTorrent" }

func (e *EliteTorrent) base() string {
	if e.Base == "" {
		return EliteTorrentBase
	}
	return strings.TrimSuffix(e.Base, "/")
}

// URLBusqueda arma la URL de búsqueda. Es un WordPress de toda la vida: la
// consulta va en ?s= y no hay más parámetros que valgan.
func (e *EliteTorrent) URLBusqueda(consulta string) string {
	v := url.Values{"s": {consulta}}
	return e.base() + "/?" + v.Encode()
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
	base, err := url.Parse(e.base())
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
	doc, err := e.Cliente.Documento(ctx, r.Ficha)
	if err != nil {
		return fmt.Errorf("elitetorrent: %w", err)
	}
	return e.parsearFicha(doc, r)
}

func (e *EliteTorrent) parsearFicha(doc *goquery.Document, r *Resultado) error {
	base, err := url.Parse(e.base())
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

// absoluta convierte un enlace relativo en absoluto. Estos sitios mezclan las
// dos formas dentro de la misma página.
func absoluta(base *url.URL, href string) string {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return href
	}
	return base.ResolveReference(u).String()
}
