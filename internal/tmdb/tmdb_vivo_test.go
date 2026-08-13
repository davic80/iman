//go:build vivo

// Estos tests sí salen a internet y hablan con el TMDB de verdad. No corren en
// CI: van con `go test -tags vivo ./internal/tmdb/` y hace falta la clave.
//
//	IMAN_TMDB=... go test -tags vivo -v ./internal/tmdb/
//
// Sirven para dos cosas: comprobar que la clave vale y comprobar que la
// respuesta de TMDB sigue teniendo la forma que dan por buena las capturas de
// testdata. Cuando estos fallen y los otros pasen, es que TMDB ha cambiado.
package tmdb

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/davic80/iman/internal/titulos"
)

func clienteVivo(t *testing.T) *Cliente {
	t.Helper()
	token := os.Getenv("IMAN_TMDB")
	if token == "" {
		t.Skip("sin IMAN_TMDB no hay a quién preguntar")
	}
	return Nuevo(token, silencio())
}

func TestVivoEncuentraUnaPelicula(t *testing.T) {
	c := clienteVivo(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()

	f, hay := c.Buscar(ctx, titulos.Analizar("Matrix 1999 1080p Castellano"))
	if !hay {
		t.Fatal("TMDB no reconoció Matrix: o la clave no vale o ha cambiado la respuesta")
	}
	if f.ID != 603 {
		t.Errorf("ID = %d, quiero 603", f.ID)
	}
	if f.Titulo != "Matrix" {
		t.Errorf("Titulo = %q, quiero el castellano (TMDB ya no contesta en es-ES?)", f.Titulo)
	}
	if f.Año != 1999 || f.Cartel == "" || f.Sinopsis == "" {
		t.Errorf("ficha a medias: %+v", f)
	}
	t.Logf("ficha: %+v", f)
}

func TestVivoElAñoDistingueLosRemakes(t *testing.T) {
	c := clienteVivo(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()

	vieja, hay := c.Buscar(ctx, titulos.Analizar("Dune 1984 DVDRip"))
	if !hay {
		t.Fatal("no encontró Dune del 84")
	}
	nueva, hay := c.Buscar(ctx, titulos.Analizar("Dune 2021 1080p"))
	if !hay {
		t.Fatal("no encontró Dune de 2021")
	}
	if vieja.ID == nueva.ID {
		t.Errorf("las dos Dune salieron con la misma ficha (%d)", vieja.ID)
	}
}

func TestVivoEncuentraUnaSerie(t *testing.T) {
	c := clienteVivo(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()

	f, hay := c.Buscar(ctx, titulos.Analizar("Breaking Bad 1x02 720p"))
	if !hay {
		t.Fatal("no encontró la serie")
	}
	if f.ID != 1396 {
		t.Errorf("ID = %d, quiero 1396", f.ID)
	}
}

func TestVivoTraeLaCaratula(t *testing.T) {
	c := clienteVivo(t)
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()

	f, hay := c.Buscar(ctx, titulos.Analizar("Matrix 1999 1080p"))
	if !hay {
		t.Fatal("sin ficha no hay carátula que pedir")
	}

	cuerpo, tipo, err := c.Cartel(ctx, f.Cartel)
	if err != nil {
		t.Fatalf("no se pudo traer la carátula: %v", err)
	}
	defer cuerpo.Close()

	n, err := io.Copy(io.Discard, cuerpo)
	if err != nil {
		t.Fatalf("se cortó la descarga: %v", err)
	}
	if n < 1024 {
		t.Errorf("la carátula pesa %d bytes: eso no es una imagen", n)
	}
	t.Logf("carátula %s: %s, %d bytes", f.Cartel, tipo, n)
}
