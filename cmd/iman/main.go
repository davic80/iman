// Comando iman: el servidor web.
//
// Aquí se monta todo: los conectores, el buscador que los lanza en paralelo y
// el vigilante que mantiene a cada sitio apuntando al dominio en el que vive
// hoy. Nada de esto se descubre solo, va todo escrito a mano en `motor`.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/dominios"
	"github.com/davic80/iman/internal/novedades"
	"github.com/davic80/iman/internal/tmdb"
	"github.com/davic80/iman/internal/web"
)

// version la inyecta el Dockerfile con -ldflags. En local vale "dev".
var version = "dev"

func main() {
	sonda := flag.Bool("sonda", false,
		"comprueba que el servidor responde y termina (para el HEALTHCHECK)")
	flag.Parse()

	if *sonda {
		if err := comprobar(); err != nil {
			fmt.Fprintln(os.Stderr, "sonda:", err)
			os.Exit(1)
		}
		return
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := ejecutar(log); err != nil {
		log.Error("arranque fallido", "error", err)
		os.Exit(1)
	}
}

// motor arma el buscador, el resolutor de dominios y el rondín de novedades con
// los sitios conectados.
//
// El cliente HTTP es uno solo y compartido: el freno que espacia las peticiones
// va por dominio, así que sitios distintos no se estorban, pero dos conectores
// del mismo sitio sí se ponen en fila. Eso incluye al resolutor y al rondín, que
// así no pueden atropellar a un sitio mientras alguien busca en él.
func motor(log *slog.Logger, cfg web.Config) (*buscador.Buscador, *dominios.Resolutor, *novedades.Rondin) {
	cliente := conectores.NuevoCliente(2 * time.Second)
	elite := conectores.NuevoEliteTorrent(cliente)
	// DonTorrent veta las IPs de centros de datos, así que puede salir por un
	// proxy aparte. Con su propio cliente no comparte freno con los demás, que
	// tampoco hace falta: el freno es por dominio.
	clienteDon := cliente
	if cfg.ProxyDonTorrent != "" {
		if p, err := url.Parse(cfg.ProxyDonTorrent); err != nil || p.Host == "" {
			log.Warn("proxy de DonTorrent ilegible, sale directo", "proxy", cfg.ProxyDonTorrent)
		} else {
			clienteDon = cliente.ConProxy(p)
			log.Info("DonTorrent sale por proxy", "proxy", p.Host)
		}
	}
	don := conectores.NuevoDonTorrent(clienteDon)
	divx := conectores.NuevoDivxTotal(cliente)

	// Que no se pueda guardar el estado no impide arrancar: solo significa que
	// el siguiente arranque volverá a descubrir los dominios.
	estado, err := dominios.CargarEstado(cfg.EstadoPath)
	if err != nil {
		log.Warn("no se pudo leer el estado guardado", "ruta", cfg.EstadoPath, "error", err)
	}

	vigilante := dominios.Nuevo(cliente, log, estado, elite, don, divx)
	vigilante.Restaurar()

	rondin := novedades.Nuevo(log, novedades.NuevoAlmacen(cfg.NovedadesPath), elite, don, divx)
	rondin.Cada = cfg.RondaNovedades
	rondin.Restaurar()

	return buscador.Nuevo(log, cfg.TiempoBusqueda, elite, don, divx), vigilante, rondin
}

func ejecutar(log *slog.Logger) error {
	cfg := web.CargarConfig(version)

	busca, vigilante, rondin := motor(log, cfg)
	servidor, err := web.Nuevo(cfg, log, busca)
	if err != nil {
		return err
	}
	servidor.ConVigilante(vigilante)
	servidor.ConNovedades(rondin)

	// Sin IMAN_TMDB esto queda apagado y no se habla con nadie: se dice en el
	// log para que no haya que adivinar por qué la portada sale sin carátulas.
	carteles := tmdb.Nuevo(cfg.TMDB, log)
	servidor.ConTMDB(carteles)
	log.Info("carátulas", "tmdb", carteles.Activo())
	if cfg.GoogleID == "" {
		log.Warn("acceso abierto: sin IMAN_GOOGLE_ID no se pide cuenta de Google")
	} else {
		log.Info("acceso con google", "permitido", cfg.Permitido, "url", cfg.URLPublica)
	}

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: servidor.Handler(),
		// Los sitios que scrapeamos son lentos, pero nuestros clientes no
		// deberian serlo: quien nos habla es Caddy, en la misma maquina.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Ctrl-C y el SIGTERM que manda `docker compose down` acaban aqui.
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	// El vigilante de dominios vive aparte del servidor y nunca en el camino de
	// una búsqueda: cuando un sitio se muda, quien lo descubre es él, en
	// segundo plano, y la búsqueda siguiente ya sale bien.
	go vigilante.Vigilar(ctx)

	// El rondín de novedades, igual: la portada se sirve siempre de lo apuntado,
	// así que entrar en ella no dispara ninguna petición a ningún sitio.
	go rondin.Vigilar(ctx)

	errServidor := make(chan error, 1)
	go func() {
		log.Info("escuchando", "addr", cfg.Addr, "version", cfg.Version)
		errServidor <- srv.ListenAndServe()
	}()

	select {
	case err := <-errServidor:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		log.Info("cerrando")
	}

	// Margen para que las peticiones en vuelo terminen antes de morir.
	cierre, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	return srv.Shutdown(cierre)
}

// comprobar es el modo sonda. La imagen es un distroless sin shell ni curl, asi
// que el healthcheck del contenedor lo hace el propio binario contra si mismo.
func comprobar() error {
	cfg := web.CargarConfig(version)

	// cfg.Addr suele ser ":8080"; para hablar con nosotros mismos hace falta
	// un host explicito.
	host, puerto, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return fmt.Errorf("direccion %q ilegible: %w", cfg.Addr, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}

	cliente := &http.Client{Timeout: 3 * time.Second}
	url := fmt.Sprintf("http://%s/vivo", net.JoinHostPort(host, puerto))

	resp, err := cliente.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s devolvio %d", url, resp.StatusCode)
	}
	return nil
}
