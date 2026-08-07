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

// DonTorrentBase es el dominio por el que se empieza.
//
// No es dontorrent.management, que es el dominio que el sitio anuncia como
// oficial: ese tiene vetado en Cloudflare el ASN entero de Hetzner y contesta
// "error code: 1005" a cualquier petición desde el servidor. TomaDivx es el
// mismo sitio, la misma plantilla y el mismo catálogo, y sí contesta.
const DonTorrentBase = "https://tomadivx.net"

// DonTorrent es la plataforma de DonTorrent, que vive a la vez en varios
// dominios con marcas distintas (TomaDivx, NaranjaTorrent...). Todos sirven el
// mismo HTML, así que un solo parser vale para cualquiera.
//
// Sus dos manías: exige un Referer suyo para dejarte buscar, y no dice el
// idioma en ninguna parte. Lo segundo se resuelve en idiomaPorDefecto.
type DonTorrent struct {
	Cliente *Cliente

	mu   sync.RWMutex
	base string
}

// NuevoDonTorrent crea el conector.
func NuevoDonTorrent(c *Cliente) *DonTorrent {
	if c == nil {
		c = NuevoCliente(2 * time.Second)
	}
	return &DonTorrent{Cliente: c, base: DonTorrentBase}
}

// Si esto deja de compilar es que el conector ya no cumple el contrato.
var (
	_ Mudable     = (*DonTorrent)(nil)
	_ Resolutor   = (*DonTorrent)(nil)
	_ Descargador = (*DonTorrent)(nil)
)

func (d *DonTorrent) Nombre() string { return "DonTorrent" }

// Dominio es el host en uso, para enseñarlo en /salud.
func (d *DonTorrent) Dominio() string {
	u, err := url.Parse(d.Base())
	if err != nil {
		return ""
	}
	return u.Host
}

func (d *DonTorrent) Base() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.base == "" {
		return DonTorrentBase
	}
	return strings.TrimSuffix(d.base, "/")
}

func (d *DonTorrent) Mudar(base string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.base = strings.TrimSuffix(base, "/")
}

// Semillas van en orden de utilidad, no de oficialidad. Los dos espejos que
// responden desde el servidor primero; dontorrent.management al final porque,
// aunque es el dominio que el sitio anuncia, desde Hetzner solo devuelve el
// veto de ASN. Si algún día se levanta, el resolutor lo encontrará ahí.
func (d *DonTorrent) Semillas() []string {
	return []string{
		DonTorrentBase,
		"https://naranjatorrent.com",
		"https://dontorrent.management",
	}
}

// Huella son marcas de la plantilla, nunca de la marca comercial.
//
// Es a propósito: este sitio vive en muchos dominios con nombres distintos, y
// justamente la palabra "dontorrent" no aparece en el HTML de los espejos...
// pero sí aparece cinco veces en el parking de dontorrent.click. Buscarla como
// marca positiva sería premiar al impostor y castigar al bueno.
func (d *DonTorrent) Huella() Huella {
	return Huella{
		Contiene: []string{"descargar-peliculas", "se han encontrado"},
		Consulta: "matrix",
		// La búsqueda de "matrix" da 13 resultados en una página. Tres es
		// margen de sobra para distinguir un sitio vivo de uno que finge.
		MinAciertos: 3,
	}
}

// Sondear hace la búsqueda de prueba contra un candidato sin tocar la base en
// uso, para que el vigilante pueda investigar mientras alguien busca.
func (d *DonTorrent) Sondear(ctx context.Context, base string) (Sondeo, error) {
	sonda := &DonTorrent{Cliente: d.Cliente, base: base}

	doc, err := sonda.pedirBusqueda(ctx, sonda.Huella().Consulta)
	if err != nil {
		return Sondeo{}, err
	}
	html, err := doc.Html()
	if err != nil {
		return Sondeo{}, fmt.Errorf("no se pudo leer el HTML: %w", err)
	}
	rs, err := sonda.parsearBusqueda(doc)
	if err != nil {
		return Sondeo{HTML: html}, err
	}
	return Sondeo{HTML: html, Resultados: len(rs)}, nil
}

// URLBusqueda arma la URL de búsqueda. La consulta va en la ruta, no en un
// parámetro: /buscar/matrix.
func (d *DonTorrent) URLBusqueda(consulta string) string {
	return d.Base() + "/buscar/" + url.PathEscape(strings.TrimSpace(consulta))
}

func (d *DonTorrent) Buscar(ctx context.Context, consulta string) ([]Resultado, error) {
	doc, err := d.pedirBusqueda(ctx, consulta)
	if err != nil {
		return nil, err
	}
	return d.parsearBusqueda(doc)
}

// pedirBusqueda pide la página de resultados con el Referer que el sitio exige.
//
// Sin esa cabecera responde 200 con "Necesitas utilizar el buscador" y ni un
// resultado, que es su manera de pedir que hayas pasado por el formulario.
func (d *DonTorrent) pedirBusqueda(ctx context.Context, consulta string) (*goquery.Document, error) {
	doc, err := d.Cliente.DocumentoDesde(ctx, d.URLBusqueda(consulta), d.Base()+"/")
	if err != nil {
		return nil, fmt.Errorf("dontorrent: %w", err)
	}
	return doc, nil
}

// parsearBusqueda saca los resultados. Cada uno es un <p> dentro de #buscador
// con el enlace a la ficha, un <span> con el formato y una etiqueta de sección.
func (d *DonTorrent) parsearBusqueda(doc *goquery.Document) ([]Resultado, error) {
	base, err := url.Parse(d.Base())
	if err != nil {
		return nil, fmt.Errorf("dontorrent: base inválida: %w", err)
	}

	var out []Resultado
	doc.Find("#buscador p").Each(func(_ int, p *goquery.Selection) {
		enlace := p.Find("a.text-decoration-none").First()
		titulo := strings.TrimSpace(enlace.Text())
		href, ok := enlace.Attr("href")
		if titulo == "" || !ok {
			return
		}

		r := Resultado{
			Sitio:  d.Nombre(),
			Titulo: titulo,
			Ficha:  absoluta(base, href),
			// Este sitio no publica ni semillas ni clientes en ninguna parte,
			// y -1 es como se dice "no se sabe" en vez de "no hay nadie".
			Semillas: -1,
			Clientes: -1,
			Info:     titulos.Analizar(titulo),
		}

		// El formato va en un <span> suelto al lado del enlace: "(4K)",
		// "(BDremux-1080p)". Es más fiable que deducirlo del título.
		if c := titulos.DetectarCalidad(formatoDeFila(p)); c != titulos.CalidadDesconocida {
			r.Info.Calidad = c
		}
		r.Info.Idioma = idiomaPorDefecto(r.Info.Idioma)

		out = append(out, r)
	})
	return out, nil
}

// idiomaPorDefecto rellena el idioma cuando el sitio no dice nada.
//
// DonTorrent no etiqueta el idioma en ningún sitio, pero sí tiene convención:
// lo que no es castellano lo avisa en el propio título ("Cartas a Julieta
// (Latino)", "American Assassin [Audio Español-Latino]"). Sobre 21.655
// películas, buscar "latino" devuelve 92 resultados y todos llevan la marca
// escrita. Así que aquí la ausencia de marca sí significa castellano.
//
// Solo se aplica cuando el título no ha dicho nada: si el título trae marca,
// manda el título y este conector no la pisa.
//
// Lo usa también DivxTotal, que sí etiqueta el idioma pero solo en la ficha, y
// la ficha no se pide para pintar la lista.
func idiomaPorDefecto(i titulos.Idioma) titulos.Idioma {
	if i == titulos.Desconocido {
		return titulos.Castellano
	}
	return i
}

// formatoDeFila devuelve el texto del <span> que acompaña al enlace, que es
// donde va el formato. Se distingue del de la sección ("Película", "Serie")
// porque ese lleva clase badge.
func formatoDeFila(p *goquery.Selection) string {
	var formato string
	p.Find("span").Each(func(_ int, s *goquery.Selection) {
		if s.HasClass("badge") || s.Find("a").Length() > 0 {
			return
		}
		if t := strings.TrimSpace(s.Text()); t != "" {
			formato = t
		}
	})
	return formato
}

// Resolver pide la ficha para sacar el .torrent, que en la lista no viene.
//
// Este sitio no da magnets: sirve el fichero .torrent directamente desde su
// propio dominio.
func (d *DonTorrent) Resolver(ctx context.Context, r *Resultado) error {
	if r.Ficha == "" {
		return fmt.Errorf("dontorrent: resultado sin ficha")
	}
	if err := d.esMia(r.Ficha); err != nil {
		return err
	}
	doc, err := d.Cliente.DocumentoDesde(ctx, r.Ficha, d.Base()+"/")
	if err != nil {
		return fmt.Errorf("dontorrent: %w", err)
	}
	return d.parsearFicha(doc, r)
}

func (d *DonTorrent) parsearFicha(doc *goquery.Document, r *Resultado) error {
	base, err := url.Parse(d.Base())
	if err != nil {
		return fmt.Errorf("dontorrent: base inválida: %w", err)
	}

	// El botón de descarga es el único <a> con atributo download.
	if t, ok := doc.Find("a[download]").First().Attr("href"); ok {
		r.Torrent = absoluta(base, t)
	}
	if r.Torrent == "" {
		return fmt.Errorf("dontorrent: ficha %s no tiene enlace de descarga", r.Ficha)
	}

	for etiqueta, valor := range fichaDonTorrent(doc) {
		switch etiqueta {
		case "tamaño":
			if t := ParsearTamaño(valor); t > 0 {
				r.Tamaño = t
			}
		case "año":
			if a := titulos.Analizar(valor).Año; a > 0 {
				r.Info.Año = a
			}
		case "formato":
			if c := titulos.DetectarCalidad(valor); c != titulos.CalidadDesconocida {
				r.Info.Calidad = c
			}
		}
	}
	r.Info.Idioma = idiomaPorDefecto(r.Info.Idioma)
	return nil
}

// fichaDonTorrent lee la tabla de datos de la ficha, que son <p> con un <b> de
// etiqueta y el valor detrás.
func fichaDonTorrent(doc *goquery.Document) map[string]string {
	datos := map[string]string{}
	doc.Find("p").Each(func(_ int, p *goquery.Selection) {
		etiqueta := strings.TrimSpace(p.Find("b").First().Text())
		if etiqueta == "" {
			return
		}
		valor := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(p.Text()), etiqueta))
		clave := strings.ToLower(strings.TrimSuffix(etiqueta, ":"))
		if valor != "" {
			datos[clave] = valor
		}
	})
	return datos
}

// Descargar trae el .torrent. Lo pide el servidor, no el navegador de quien
// busca.
func (d *DonTorrent) Descargar(ctx context.Context, torrent string) (io.ReadCloser, error) {
	if err := d.esMia(torrent); err != nil {
		return nil, err
	}
	return d.Cliente.TraerDesde(ctx, torrent, d.Base()+"/")
}

// esMia comprueba que una URL es de este sitio antes de pedirla.
//
// Las URLs llegan de la petición del usuario: sin esto, Imán sería un proxy
// abierto con el que pedir cualquier cosa en su nombre, incluida la red interna
// del servidor.
func (d *DonTorrent) esMia(dir string) error {
	base, err := url.Parse(d.Base())
	if err != nil {
		return fmt.Errorf("dontorrent: base inválida: %w", err)
	}
	u, err := url.Parse(dir)
	if err != nil {
		return fmt.Errorf("dontorrent: url inválida %q: %w", dir, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("dontorrent: %q no es una url http", dir)
	}
	if !mismoSitio(u.Hostname(), base.Hostname()) {
		return fmt.Errorf("dontorrent: %q no es de este sitio", u.Hostname())
	}
	return nil
}
