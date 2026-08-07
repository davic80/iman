package conectores

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/davic80/iman/internal/titulos"
)

// DivxTotalBase es el dominio por el que se empieza.
//
// No es divxtotal.tv, que es el que el sitio usó durante años: ese lo compró un
// parking y hoy son 114 bytes que redirigen a /lander. El sitio se mudó al .foo
// y sigue vivo y actualizado.
const DivxTotalBase = "https://divxtotal.foo"

// DivxTotal es un WordPress con plantilla propia, sin anti-bot de ninguna clase.
// Busca con ?s= y declara el idioma en la ficha, que es lo que interesa aquí.
//
// Su gracia está en el enlace de descarga: el botón visible manda a un acortador
// de terceros, pero el mismo <a> lleva la URL buena en un data-src en base64.
// Ver enlaceTorrent.
type DivxTotal struct {
	Cliente *Cliente

	mu   sync.RWMutex
	base string
}

// NuevoDivxTotal crea el conector.
func NuevoDivxTotal(c *Cliente) *DivxTotal {
	if c == nil {
		c = NuevoCliente(2 * time.Second)
	}
	return &DivxTotal{Cliente: c, base: DivxTotalBase}
}

// Si esto deja de compilar es que el conector ya no cumple el contrato.
var (
	_ Mudable     = (*DivxTotal)(nil)
	_ Resolutor   = (*DivxTotal)(nil)
	_ Descargador = (*DivxTotal)(nil)
)

func (d *DivxTotal) Nombre() string { return "DivxTotal" }

// Dominio es el host en uso, para enseñarlo en /salud.
func (d *DivxTotal) Dominio() string {
	u, err := url.Parse(d.Base())
	if err != nil {
		return ""
	}
	return u.Host
}

func (d *DivxTotal) Base() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.base == "" {
		return DivxTotalBase
	}
	return strings.TrimSuffix(d.base, "/")
}

func (d *DivxTotal) Mudar(base string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.base = strings.TrimSuffix(base, "/")
}

// Semillas incluye el .tv aunque hoy sea un parking: si algún día el sitio lo
// recupera, el resolutor lo encontrará ahí, y mientras tanto la huella lo
// rechaza sola.
func (d *DivxTotal) Semillas() []string {
	return []string{DivxTotalBase, "https://divxtotal.tv"}
}

// Huella son marcas de la plantilla y del buscador, no de la marca comercial.
//
// "divxtotal" aparece por todas partes en el HTML bueno, y sería cómodo usarlo,
// pero es justo el error que enseñó DonTorrent: la palabra la puede poner
// cualquiera que quiera hacerse pasar por el sitio, y de hecho elitetorrent.one
// hace eso mismo con la marca de EliteTorrent. Estas dos, en cambio, describen
// lo que el sitio *hace*.
func (d *DivxTotal) Huella() Huella {
	return Huella{
		Contiene: []string{"torrents encontrados", "table-archive"},
		Consulta: "matrix",
		// La búsqueda de "matrix" da 23 resultados repartidos en páginas de 15.
		// Tres es margen de sobra para distinguir un sitio vivo de uno que finge.
		MinAciertos: 3,
	}
}

// Sondear hace la búsqueda de prueba contra un candidato sin tocar la base en
// uso, para que el vigilante pueda investigar mientras alguien busca.
func (d *DivxTotal) Sondear(ctx context.Context, base string) (Sondeo, error) {
	sonda := &DivxTotal{Cliente: d.Cliente, base: base}

	doc, err := sonda.Cliente.Documento(ctx, sonda.URLBusqueda(sonda.Huella().Consulta))
	if err != nil {
		return Sondeo{}, fmt.Errorf("divxtotal: %w", err)
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

// URLBusqueda arma la URL de búsqueda: el ?s= de toda la vida de WordPress.
func (d *DivxTotal) URLBusqueda(consulta string) string {
	v := url.Values{"s": {strings.TrimSpace(consulta)}}
	return d.Base() + "/?" + v.Encode()
}

func (d *DivxTotal) Buscar(ctx context.Context, consulta string) ([]Resultado, error) {
	doc, err := d.Cliente.Documento(ctx, d.URLBusqueda(consulta))
	if err != nil {
		return nil, fmt.Errorf("divxtotal: %w", err)
	}
	return d.parsearBusqueda(doc)
}

// parsearBusqueda lee la tabla de resultados: nombre, categoría, fecha y peso.
//
// El peso casi siempre viene "N/A" y el idioma no aparece; los dos salen de la
// ficha. Lo que sí sirve de aquí es el título y el enlace.
func (d *DivxTotal) parsearBusqueda(doc *goquery.Document) ([]Resultado, error) {
	base, err := url.Parse(d.Base())
	if err != nil {
		return nil, fmt.Errorf("divxtotal: base inválida: %w", err)
	}

	var out []Resultado
	doc.Find("table.table tbody tr").Each(func(_ int, tr *goquery.Selection) {
		celdas := tr.Find("td")
		if celdas.Length() < 2 {
			return
		}

		enlace := celdas.Eq(0).Find("a").First()
		titulo := strings.TrimSpace(enlace.Text())
		href, ok := enlace.Attr("href")
		if titulo == "" || !ok {
			return
		}

		r := Resultado{
			Sitio:  d.Nombre(),
			Titulo: titulo,
			Ficha:  absoluta(base, href),
			// Este sitio no publica ni semillas ni clientes en ninguna parte.
			Semillas: -1,
			Clientes: -1,
			Info:     titulos.Analizar(titulo),
		}

		// En la lista no hay ni idioma ni bandera: eso solo sale en la ficha,
		// que no se pide hasta que alguien quiere descargar. Así que aquí vale
		// la convención del sitio, la misma que en DonTorrent: lo que no es
		// castellano lo lleva escrito en el título. Sin esto la búsqueda entera
		// se cae en el filtro de idioma por venir en blanco.
		r.Info.Idioma = idiomaPorDefecto(r.Info.Idioma)

		// Las dos últimas columnas son fecha y peso, en ese orden, y las dos
		// suelen venir vacías o con "N/A".
		if celdas.Length() >= 4 {
			if f, err := time.Parse("02-01-2006", textoCelda(celdas.Eq(2))); err == nil {
				r.Fecha = f
			}
			if t := ParsearTamaño(textoCelda(celdas.Eq(3))); t > 0 {
				r.Tamaño = t
			}
		}

		out = append(out, r)
	})
	return out, nil
}

// textoCelda saca el texto de una celda limpio de espacios y del &nbsp; que la
// plantilla mete por todas partes. goquery ya ha decodificado la entidad, así
// que lo que llega aquí es el carácter U+00A0, que TrimSpace sí quita.
func textoCelda(td *goquery.Selection) string {
	return strings.TrimSpace(td.Text())
}

// Resolver pide la ficha para sacar el .torrent y el idioma, que en la lista no
// vienen.
func (d *DivxTotal) Resolver(ctx context.Context, r *Resultado) error {
	if r.Ficha == "" {
		return fmt.Errorf("divxtotal: resultado sin ficha")
	}
	if err := d.esMia(r.Ficha); err != nil {
		return err
	}
	doc, err := d.Cliente.Documento(ctx, r.Ficha)
	if err != nil {
		return fmt.Errorf("divxtotal: %w", err)
	}
	return d.parsearFicha(doc, r)
}

func (d *DivxTotal) parsearFicha(doc *goquery.Document, r *Resultado) error {
	base, err := url.Parse(d.Base())
	if err != nil {
		return fmt.Errorf("divxtotal: base inválida: %w", err)
	}

	t, err := enlaceTorrent(doc)
	if err != nil {
		return fmt.Errorf("divxtotal: ficha %s: %w", r.Ficha, err)
	}
	r.Torrent = absoluta(base, t)

	// Este sitio no da magnets, solo el fichero.
	for etiqueta, valor := range fichaDivxTotal(doc) {
		switch etiqueta {
		case "idioma":
			// Lo declara el sitio, así que manda sobre lo que diga el título:
			// distingue "Español" de "Español Latino", que es la diferencia que
			// a Imán le importa.
			if i := titulos.DetectarIdioma(valor); i != titulos.Desconocido {
				r.Info.Idioma = i
			}
		case "calidad":
			if c := titulos.DetectarCalidad(valor); c != titulos.CalidadDesconocida {
				r.Info.Calidad = c
			}
		case "fecha":
			if f, err := time.Parse("02-01-2006", valor); err == nil && r.Fecha.IsZero() {
				r.Fecha = f
			}
		}
	}
	return nil
}

// enlaceTorrent saca la URL del .torrent, que es la parte con truco del sitio.
//
// El botón "Descargar" apunta a un acortador de terceros (short-info.link), y
// ese no vale: Imán promete darte el enlace, no mandarte por una cadena de
// anuncios. Pero el mismo <a> lleva un data-src con la URL de verdad en base64,
// alojada en el propio dominio del sitio. Es la que se usa.
//
// En una ficha de serie hay una fila por capítulo y todas tienen su data-src;
// se coge la primera, igual que en los demás conectores un resultado es un
// enlace. Agrupar los capítulos de una serie es cosa de la fase 4.
func enlaceTorrent(doc *goquery.Document) (string, error) {
	var codificado string
	doc.Find("a.linktorrent[data-src]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		if v, ok := a.Attr("data-src"); ok && strings.TrimSpace(v) != "" {
			codificado = strings.TrimSpace(v)
			return false
		}
		return true
	})
	if codificado == "" {
		return "", fmt.Errorf("no tiene enlace de descarga")
	}

	crudo, err := base64.StdEncoding.DecodeString(codificado)
	if err != nil {
		return "", fmt.Errorf("el data-src no es base64: %w", err)
	}
	dir := strings.TrimSpace(string(crudo))
	if dir == "" {
		return "", fmt.Errorf("el data-src decodifica a vacío")
	}
	return dir, nil
}

// fichaDivxTotal lee el bloque de datos de la ficha, que son pares de <p>: uno
// con la clase tagitem y la etiqueta, y el siguiente con el valor.
//
// La etiqueta viene como "Idioma:&nbsp;". goquery decodifica la entidad a
// U+00A0, así que basta TrimSpace, que sí lo considera espacio, y quitar los
// dos puntos.
func fichaDivxTotal(doc *goquery.Document) map[string]string {
	datos := map[string]string{}
	doc.Find("p.tagitem").Each(func(_ int, p *goquery.Selection) {
		etiqueta := strings.TrimSpace(p.Text())
		etiqueta = strings.TrimSpace(strings.TrimSuffix(etiqueta, ":"))
		if etiqueta == "" {
			return
		}
		valor := strings.TrimSpace(p.Next().Text())
		if valor != "" {
			datos[strings.ToLower(etiqueta)] = valor
		}
	})
	return datos
}

// Descargar trae el .torrent. Lo pide el servidor, no el navegador de quien
// busca.
func (d *DivxTotal) Descargar(ctx context.Context, torrent string) (io.ReadCloser, error) {
	if err := d.esMia(torrent); err != nil {
		return nil, err
	}
	return d.Cliente.TraerDesde(ctx, torrent, d.Base()+"/")
}

// esMia comprueba que una URL es de este sitio antes de pedirla.
//
// Las URLs llegan de la petición del usuario: sin esto, Imán sería un proxy
// abierto con el que pedir cualquier cosa en su nombre, incluida la red interna
// del servidor. Aquí importa el doble, porque el enlace de descarga sale de un
// base64 de la página: lo que decodifique tiene que pasar el mismo filtro.
func (d *DivxTotal) esMia(dir string) error {
	base, err := url.Parse(d.Base())
	if err != nil {
		return fmt.Errorf("divxtotal: base inválida: %w", err)
	}
	u, err := url.Parse(dir)
	if err != nil {
		return fmt.Errorf("divxtotal: url inválida %q: %w", dir, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("divxtotal: %q no es una url http", dir)
	}
	if !mismoSitio(u.Hostname(), base.Hostname()) {
		return fmt.Errorf("divxtotal: %q no es de este sitio", u.Hostname())
	}
	return nil
}
