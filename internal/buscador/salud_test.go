package buscador

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
)

func TestSaludEmpiezaSinDatos(t *testing.T) {
	b := Nuevo(mudo(), time.Second, &falso{nombre: "Sitio"})

	salud := b.Salud()
	if len(salud) != 1 {
		t.Fatalf("%d conectores en salud, quiero 1", len(salud))
	}
	// Antes de preguntar nada no se sabe si el sitio está vivo, y decir que sí
	// sería inventárselo.
	if got := salud[0].Estado(); got != SinDatos {
		t.Errorf("Estado = %q, quiero %q", got, SinDatos)
	}
}

func TestSaludVivoTrasUnaBusqueda(t *testing.T) {
	sitio := &falso{nombre: "Sitio", resultado: []conectores.Resultado{
		res("Sitio", "peli", titulos.Castellano, titulos.Cal1080p),
	}}
	b := Nuevo(mudo(), time.Second, sitio)
	b.Buscar(context.Background(), "peli", Opciones{})

	s := saludDe(t, b, "Sitio")
	if s.Estado() != Vivo {
		t.Errorf("Estado = %q, quiero %q", s.Estado(), Vivo)
	}
	if s.Resultados != 1 {
		t.Errorf("Resultados = %d, quiero 1", s.Resultados)
	}
	if s.UltimaVez.IsZero() {
		t.Error("UltimaVez sin rellenar")
	}
}

// Un timeout suelto es normal en estos sitios. Pintarlos de rojo a la primera
// llenaría /salud de alarmas que no significan nada.
func TestUnFalloSueltoEsDegradadoNoCaido(t *testing.T) {
	sitio := &falso{nombre: "Sitio", err: errors.New("timeout")}
	b := Nuevo(mudo(), time.Second, sitio)

	b.Buscar(context.Background(), "peli", Opciones{})
	if got := saludDe(t, b, "Sitio").Estado(); got != Degradado {
		t.Errorf("tras 1 fallo Estado = %q, quiero %q", got, Degradado)
	}

	for range fallosParaCaido - 1 {
		b.Buscar(context.Background(), "peli", Opciones{})
	}
	s := saludDe(t, b, "Sitio")
	if s.Estado() != Caido {
		t.Errorf("tras %d fallos Estado = %q, quiero %q", fallosParaCaido, s.Estado(), Caido)
	}
	if s.Error == "" {
		t.Error("un sitio caído debería decir por qué")
	}
}

// Lo que interesa es si el sitio está roto ahora, no su historial.
func TestUnAciertoBorraLosFallos(t *testing.T) {
	sitio := &falso{nombre: "Sitio", err: errors.New("503")}
	b := Nuevo(mudo(), time.Second, sitio)

	for range fallosParaCaido {
		b.Buscar(context.Background(), "peli", Opciones{})
	}
	if got := saludDe(t, b, "Sitio").Estado(); got != Caido {
		t.Fatalf("Estado = %q, quiero %q", got, Caido)
	}

	sitio.err = nil
	sitio.resultado = []conectores.Resultado{res("Sitio", "peli", titulos.Castellano, titulos.Cal720p)}
	b.Buscar(context.Background(), "peli", Opciones{})

	s := saludDe(t, b, "Sitio")
	if s.Estado() != Vivo {
		t.Errorf("Estado = %q, quiero %q tras volver a funcionar", s.Estado(), Vivo)
	}
	if s.Error != "" {
		t.Errorf("Error = %q, debería haberse limpiado", s.Error)
	}
}

func TestCuentaLasBusquedas(t *testing.T) {
	b := Nuevo(mudo(), time.Second, &falso{nombre: "Sitio"})
	for range 3 {
		b.Buscar(context.Background(), "peli", Opciones{})
	}
	if got := b.Busquedas(); got != 3 {
		t.Errorf("Busquedas = %d, quiero 3", got)
	}
}

// /salud se recarga a mano y sin orden fijo las filas bailarían entre recargas
// sin que hubiera cambiado nada.
func TestSaludSaleEnOrdenFijo(t *testing.T) {
	b := Nuevo(mudo(), time.Second,
		&falso{nombre: "Zeta"}, &falso{nombre: "Alfa"}, &falso{nombre: "Media"})

	for range 3 {
		salud := b.Salud()
		if len(salud) != 3 {
			t.Fatalf("%d conectores, quiero 3", len(salud))
		}
		for i, quiero := range []string{"Alfa", "Media", "Zeta"} {
			if salud[i].Nombre != quiero {
				t.Fatalf("salud[%d] = %q, quiero %q", i, salud[i].Nombre, quiero)
			}
		}
	}
}
