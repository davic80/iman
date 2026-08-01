// Comando iman: el servidor web.
//
// Fase 0 — todavia no hay conectores ni busqueda. Lo que hay es el tubo
// completo: binario, imagen, CI, compose y Caddy. Se despliega en vacio a
// proposito, para no tener que depurar el despliegue y el scraping a la vez.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

func ejecutar(log *slog.Logger) error {
	cfg := web.CargarConfig(version)

	servidor, err := web.Nuevo(cfg, log, nil)
	if err != nil {
		return err
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
