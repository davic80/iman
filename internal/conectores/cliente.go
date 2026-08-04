package conectores

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// UAPorDefecto es el User-Agent con el que se piden las páginas.
//
// Con el User-Agent que pone Go por defecto varios de estos sitios contestan
// 403 o directamente cuelgan la conexión. No es para esconderse: es que
// esperan un navegador porque es lo único que les visita.
const UAPorDefecto = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// MaxCuerpo es cuánto se lee como mucho de una respuesta. Una página de
// resultados ocupa unos 50 KB; el límite está para que un sitio que se ponga a
// escupir megas no se lleve por delante la memoria del proceso.
const MaxCuerpo = 4 << 20 // 4 MiB

// Cliente es un cliente HTTP que se porta bien con los sitios: espacia las
// peticiones a un mismo dominio y no se traga respuestas enormes.
//
// Espaciarlas no es cortesía teórica. EliteTorrent empezó a dar timeouts
// mientras estudiábamos su HTML, y era por pedirle varias páginas seguidas.
type Cliente struct {
	http      *http.Client
	ua        string
	intervalo time.Duration

	mu     sync.Mutex
	ultima map[string]time.Time // Cuándo toca la siguiente petición a cada host
}

// NuevoCliente crea un cliente que deja pasar como poco `intervalo` entre dos
// peticiones al mismo host. Distintos hosts no se estorban entre ellos.
func NuevoCliente(intervalo time.Duration) *Cliente {
	return &Cliente{
		http: &http.Client{
			Timeout: 20 * time.Second,
			// Diez saltos son de sobra para los redirects normales de estos
			// sitios (http→https, dominio viejo→nuevo) y cortan los bucles.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("demasiados redirects")
				}
				return nil
			},
		},
		ua:        UAPorDefecto,
		intervalo: intervalo,
		ultima:    map[string]time.Time{},
	}
}

// Documento pide una URL y devuelve el HTML ya parseado.
func (c *Cliente) Documento(ctx context.Context, dir string) (*goquery.Document, error) {
	cuerpo, err := c.Traer(ctx, dir)
	if err != nil {
		return nil, err
	}
	defer cuerpo.Close()

	doc, err := goquery.NewDocumentFromReader(cuerpo)
	if err != nil {
		return nil, fmt.Errorf("parseando %s: %w", dir, err)
	}
	return doc, nil
}

// Traer pide una URL y devuelve el cuerpo. Hay que cerrarlo.
func (c *Cliente) Traer(ctx context.Context, dir string) (io.ReadCloser, error) {
	u, err := url.Parse(dir)
	if err != nil {
		return nil, fmt.Errorf("url inválida %q: %w", dir, err)
	}
	if err := c.esperarTurno(ctx, u.Host); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dir, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pidiendo %s: %w", dir, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("pidiendo %s: %s", dir, resp.Status)
	}

	return &cuerpoLimitado{
		Reader: io.LimitReader(resp.Body, MaxCuerpo),
		cerrar: resp.Body,
	}, nil
}

// Destino dice en qué URL se acaba tras seguir las redirecciones.
//
// Es como se encuentra el dominio nuevo de un sitio que se ha mudado: los
// viejos suelen quedarse un tiempo redirigiendo al que funciona. El cuerpo se
// descarta, aquí solo interesa dónde se aterriza.
func (c *Cliente) Destino(ctx context.Context, dir string) (string, error) {
	u, err := url.Parse(dir)
	if err != nil {
		return "", fmt.Errorf("url inválida %q: %w", dir, err)
	}
	if err := c.esperarTurno(ctx, u.Host); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dir, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.ua)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("pidiendo %s: %w", dir, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, MaxCuerpo))

	// Request.URL es la de la última petición de la cadena, que es justo la
	// que interesa. Del resto de la URL no queremos nada: el sitio vive en el
	// dominio, no en la página a la que nos haya dejado.
	final := *resp.Request.URL
	final.Path, final.RawQuery, final.Fragment = "", "", ""
	return strings.TrimSuffix(final.String(), "/"), nil
}

// esperarTurno bloquea hasta que le toque a este host.
//
// El turno se reserva con el candado cogido y la espera se hace fuera, así dos
// peticiones simultáneas al mismo sitio se ponen en fila en vez de salir las
// dos a la vez tras esperar lo mismo.
func (c *Cliente) esperarTurno(ctx context.Context, host string) error {
	c.mu.Lock()
	ahora := time.Now()
	turno := ahora
	if siguiente, visto := c.ultima[host]; visto && siguiente.After(ahora) {
		turno = siguiente
	}
	c.ultima[host] = turno.Add(c.intervalo)
	c.mu.Unlock()

	espera := time.Until(turno)
	if espera <= 0 {
		return nil
	}
	t := time.NewTimer(espera)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type cuerpoLimitado struct {
	io.Reader
	cerrar io.Closer
}

func (c *cuerpoLimitado) Close() error { return c.cerrar.Close() }
