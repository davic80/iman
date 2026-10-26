package web

import (
	"testing"
	"time"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/conectores"
)

func filaOrden(titulo string, fecha time.Time, tamaño int64) buscador.Resultado {
	return buscador.Resultado{Resultado: conectores.Resultado{Titulo: titulo, Fecha: fecha, Tamaño: tamaño}}
}

func titulosOrden(rs []buscador.Resultado) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Titulo)
	}
	return out
}

// Lo que no dice fecha o tamaño va al final, y entre ellos se queda el orden
// de relevancia que traían.
func TestOrdenarPorFechaYTamaño(t *testing.T) {
	dia := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	base := []buscador.Resultado{
		filaOrden("sin nada 1", time.Time{}, 0),
		filaOrden("vieja", dia(1), 1<<30),
		filaOrden("sin nada 2", time.Time{}, 0),
		filaOrden("nueva", dia(20), 4<<30),
		filaOrden("media", dia(10), 2<<30),
	}

	rs := append([]buscador.Resultado(nil), base...)
	ordenarPor(rs, ordenFecha)
	if got := titulosOrden(rs); got[0] != "nueva" || got[1] != "media" || got[2] != "vieja" || got[3] != "sin nada 1" || got[4] != "sin nada 2" {
		t.Errorf("por fecha: %q", got)
	}

	rs = append([]buscador.Resultado(nil), base...)
	ordenarPor(rs, ordenTamaño)
	if got := titulosOrden(rs); got[0] != "nueva" || got[2] != "vieja" || got[3] != "sin nada 1" {
		t.Errorf("por tamaño: %q", got)
	}

	rs = append([]buscador.Resultado(nil), base...)
	ordenarPor(rs, ordenRelevancia)
	if got := titulosOrden(rs); got[0] != "sin nada 1" || got[3] != "nueva" {
		t.Errorf("relevancia no debería tocar nada: %q", got)
	}
}
