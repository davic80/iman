package web

import (
	"testing"
	"time"
)

func TestCargarConfigPorDefecto(t *testing.T) {
	cfg := CargarConfig("dev")

	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, se esperaba \":8080\"", cfg.Addr)
	}
	if cfg.TiempoBusqueda != 20*time.Second {
		t.Errorf("TiempoBusqueda = %v, se esperaba 20s", cfg.TiempoBusqueda)
	}
}

// Sin IMAN_VERSION manda la version inyectada al compilar, que es como llega el
// SHA del commit a /salud en produccion.
func TestVersionCompilada(t *testing.T) {
	if v := CargarConfig("a1b2c3d").Version; v != "a1b2c3d" {
		t.Errorf("Version = %q, se esperaba \"a1b2c3d\"", v)
	}
}

func TestCargarConfigDesdeEntorno(t *testing.T) {
	t.Setenv("IMAN_ADDR", ":9999")
	t.Setenv("IMAN_VERSION", "abc123")

	cfg := CargarConfig("dev")
	if cfg.Addr != ":9999" {
		t.Errorf("Addr = %q, se esperaba \":9999\"", cfg.Addr)
	}
	if cfg.Version != "abc123" {
		t.Errorf("Version = %q, se esperaba \"abc123\"", cfg.Version)
	}
}

func TestDuracionAceptaSegundosSueltos(t *testing.T) {
	casos := map[string]time.Duration{
		"45s":    45 * time.Second,
		"45":     45 * time.Second, // sin unidad: segundos
		"2m":     2 * time.Minute,
		"basura": 20 * time.Second, // ilegible: se queda el defecto
		"":       20 * time.Second,
	}

	for valor, quiero := range casos {
		t.Run(valor, func(t *testing.T) {
			t.Setenv("IMAN_TIEMPO_BUSQUEDA", valor)
			if d := CargarConfig("dev").TiempoBusqueda; d != quiero {
				t.Errorf("%q -> %v, se esperaba %v", valor, d, quiero)
			}
		})
	}
}
