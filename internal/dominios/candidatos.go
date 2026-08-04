// Package dominios encuentra en qué dirección vive hoy cada sitio.
//
// Es la pieza que justifica el proyecto. Los sitios de torrents cambian de
// dominio cada pocas semanas, y el dominio que abandonan no se queda vacío:
// alguien lo compra y monta un parking que responde 200 a todo, a veces
// copiando el diseño del original. Así que hay dos trabajos, y el segundo
// importa más que el primero: proponer candidatos, y demostrar que uno de ellos
// es de verdad el sitio.
package dominios

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/davic80/iman/internal/conectores"
)

// Candidato es una dirección a probar y de dónde ha salido. El origen se apunta
// para poder leer en el log cómo se encontró el dominio nuevo: si siempre
// aparece por el mismo camino, los demás sobran.
type Candidato struct {
	Base   string
	Origen string
}

// Cliente es lo que el resolutor necesita de la red. Es una interfaz para que
// los tests puedan sustituirla: la cascada tiene bastante lógica y no debería
// hacer falta levantar un servidor para probarla.
type Cliente interface {
	Destino(ctx context.Context, dir string) (string, error)
	Traer(ctx context.Context, dir string) (io.ReadCloser, error)
}

// candidatos propone direcciones donde puede estar el sitio, de la más probable
// a la más rebuscada, y sin repetir.
//
// El orden es el que ahorra peticiones: lo que ya funcionaba, lo que el
// conector conoce, y solo después salir a preguntar por ahí. Devolver muchos
// candidatos no es gratis, cada uno son dos peticiones de verificación.
func (r *Resolutor) candidatos(ctx context.Context, c conectores.Mudable, ultimoBueno string) []Candidato {
	var out []Candidato
	vistos := map[string]bool{}

	añadir := func(base, origen string) {
		base = normalizarBase(base)
		if base == "" || vistos[base] {
			return
		}
		vistos[base] = true
		out = append(out, Candidato{Base: base, Origen: origen})
	}

	// 1. El último que funcionó. El caso normal, y el que hace que arrancar el
	// proceso no cueste una ronda de descubrimiento.
	añadir(ultimoBueno, "último bueno")

	// 2. Lo que el conector trae escrito.
	for _, s := range c.Semillas() {
		añadir(s, "semilla")
	}

	// 3. Dónde acaban las semillas al seguir sus redirecciones. Un dominio
	// muerto suele apuntar al vivo durante una temporada.
	for _, s := range append([]string{ultimoBueno}, c.Semillas()...) {
		if normalizarBase(s) == "" {
			continue
		}
		destino, err := r.cliente.Destino(ctx, s)
		if err != nil {
			continue
		}
		añadir(destino, "redirección desde "+anfitrion(s))
	}

	// 4. El canal de Telegram, cuando el sitio tiene. Es la fuente más fiable
	// que existe porque la publica el propio sitio, y es pública: no hace falta
	// cuenta para leer https://t.me/s/<canal>.
	if conCanal, ok := c.(conectores.ConCanal); ok && conCanal.Canal() != "" {
		for _, d := range r.dominiosDelCanal(ctx, conCanal.Canal()) {
			añadir(d, "telegram")
		}
	}

	// 5. El contador. Algunos sitios numeran el subdominio y suben de uno en
	// uno (www43 → www44). Es feo y es exactamente lo que hacen.
	for _, base := range append([]string{ultimoBueno}, c.Semillas()...) {
		for _, siguiente := range siguientesDelContador(base, r.SaltosContador) {
			añadir(siguiente, "contador")
		}
	}

	return out
}

// reContador caza el sufijo numérico del subdominio: www43.mejortorrent.eu.
var reContador = regexp.MustCompile(`^(https?://[a-z]+?)(\d+)(\..+)$`)

// siguientesDelContador propone los n siguientes números del subdominio.
func siguientesDelContador(base string, n int) []string {
	m := reContador.FindStringSubmatch(strings.ToLower(normalizarBase(base)))
	if m == nil {
		return nil
	}
	actual, err := strconv.Atoi(m[2])
	if err != nil {
		return nil
	}
	var out []string
	for i := 1; i <= n; i++ {
		out = append(out, fmt.Sprintf("%s%d%s", m[1], actual+i, m[3]))
	}
	return out
}

// reDominioTelegram busca dominios en el HTML del canal. Los sitios anuncian
// la mudanza en texto plano ("nuevo dominio: elitetorrent.wf"), así que se cazan
// los dominios sueltos además de los enlaces.
var reDominioTelegram = regexp.MustCompile(`(?i)\b((?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,})\b`)

// dominiosDelCanal lee un canal público de Telegram y saca los dominios que
// menciona, los más recientes primero.
func (r *Resolutor) dominiosDelCanal(ctx context.Context, canal string) []string {
	cuerpo, err := r.cliente.Traer(ctx, "https://t.me/s/"+url.PathEscape(canal))
	if err != nil {
		r.log.Warn("no se pudo leer el canal de telegram", "canal", canal, "error", err)
		return nil
	}
	defer cuerpo.Close()

	crudo, err := io.ReadAll(cuerpo)
	if err != nil {
		return nil
	}
	html := string(crudo)

	var out []string
	vistos := map[string]bool{}
	for _, m := range reDominioTelegram.FindAllStringSubmatch(html, -1) {
		d := strings.ToLower(m[1])
		if vistos[d] || esRuidoDeTelegram(d) {
			continue
		}
		vistos[d] = true
		out = append(out, "https://"+d)
	}

	// Telegram sirve el canal de lo viejo a lo nuevo, y lo que interesa es el
	// anuncio más reciente: el último dominio mencionado es el vigente.
	invertir(out)
	return out
}

// esRuidoDeTelegram descarta lo que sale en cualquier página de Telegram y en
// ningún caso es el sitio que buscamos.
func esRuidoDeTelegram(d string) bool {
	for _, ruido := range []string{
		"t.me", "telegram.org", "telegram.me", "telesco.pe", "cdn-telegram.org",
		"twitter.com", "x.com", "facebook.com", "instagram.com", "youtube.com",
		"google.com", "gmail.com", "bit.ly", "w3.org", "schema.org",
	} {
		if d == ruido || strings.HasSuffix(d, "."+ruido) {
			return true
		}
	}
	// Los ficheros que se cuelan al buscar dominios en HTML: "estilo.css",
	// "logo.png". Un dominio de verdad no acaba en una extensión conocida.
	for _, ext := range []string{".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".json", ".php", ".html"} {
		if strings.HasSuffix(d, ext) {
			return true
		}
	}
	return false
}

// normalizarBase deja la dirección en la forma canónica con la que se comparan
// los candidatos: esquema, host y nada más.
func normalizarBase(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return ""
	}
	// Solo http y https. Sin esto, un candidato sacado de un HTML ajeno podría
	// traer file:// o algo peor.
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func anfitrion(base string) string {
	u, err := url.Parse(normalizarBase(base))
	if err != nil {
		return base
	}
	return u.Host
}

func invertir(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
