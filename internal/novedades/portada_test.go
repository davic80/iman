package novedades

import (
	"context"
	"testing"
	"time"

	"github.com/davic80/iman/internal/titulos"
)

// La misma película en dos sitios es una fila de la portada, no dos.
func TestLaPortadaJuntaLaMismaPeliDeDosSitios(t *testing.T) {
	uno := &falso{nombre: "Uno", rs: castellano("Uno", "El padrino 1972 DVDRip")}
	dos := &falso{nombre: "Dos", rs: castellano("Dos", "El Padrino (1972) DVDRip")}

	r := Nuevo(callado(), nil, uno, dos)
	r.Rondar(context.Background())

	filas := r.Portada(PorFecha)
	if len(filas) != 1 {
		t.Fatalf("%d filas, quiero 1: es la misma película", len(filas))
	}
	if n := len(filas[0].Sitios()); n != 2 {
		t.Errorf("la fila tiene %d sitios, quiero 2", n)
	}
}

// Que un segundo sitio suba tres días después algo que ya estaba no lo devuelve
// a lo alto de la portada.
func TestLaFilaSeQuedaConLaPrimeraVezQueSeVio(t *testing.T) {
	ahora := time.Now().UTC()

	viejo := peli("Uno", "El padrino 1972 DVDRip", titulos.Castellano)
	nuevo := peli("Dos", "El Padrino (1972) DVDRip", titulos.Castellano)

	r := Nuevo(callado(), nil)
	r.apuntes = map[string]Apunte{
		clave(viejo): {Resultado: viejo, Visto: ahora.Add(-72 * time.Hour)},
		clave(nuevo): {Resultado: nuevo, Visto: ahora},
	}

	filas := r.Portada(PorFecha)
	if len(filas) != 1 {
		t.Fatalf("%d filas, quiero 1", len(filas))
	}
	if !filas[0].Visto.Equal(ahora.Add(-72 * time.Hour)) {
		t.Errorf("Visto = %v, quiero el de la primera vez", filas[0].Visto)
	}
}

func TestSePuedeOrdenarPorCuantosSitiosLaTienen(t *testing.T) {
	ahora := time.Now().UTC()

	sola := peli("Uno", "Relleno 2020 DVDRip", titulos.Castellano)
	enDos := peli("Uno", "Estreno 2026 DVDRip", titulos.Castellano)
	enDosBis := peli("Dos", "Estreno 2026 DVDRip", titulos.Castellano)

	r := Nuevo(callado(), nil)
	r.apuntes = map[string]Apunte{
		// La que está en un solo sitio es la más reciente a propósito: así se ve
		// que el orden por sitios manda sobre la fecha.
		clave(sola):     {Resultado: sola, Visto: ahora},
		clave(enDos):    {Resultado: enDos, Visto: ahora.Add(-time.Hour)},
		clave(enDosBis): {Resultado: enDosBis, Visto: ahora.Add(-time.Hour)},
	}

	porFecha := r.Portada(PorFecha)
	if len(porFecha) != 2 || porFecha[0].Titulo != "Relleno 2020 DVDRip" {
		t.Errorf("por fecha manda %+v, quiero el relleno, que es lo último visto",
			titulosDe(porFecha))
	}

	porSitios := r.Portada(PorSitios)
	if len(porSitios) != 2 || len(porSitios[0].Sitios()) != 2 {
		t.Errorf("por sitios manda %+v, quiero la que está en dos sitios",
			titulosDe(porSitios))
	}
}

// Lo que llegue raro por la URL no puede tirar la página.
func TestUnOrdenDesconocidoCaeEnElDeSiempre(t *testing.T) {
	if o := Orden("loquesea").Valida(); o != PorFecha {
		t.Errorf("Valida() = %q, quiero %q", o, PorFecha)
	}
	if o := Orden("").Valida(); o != PorFecha {
		t.Errorf("Valida() = %q, quiero %q", o, PorFecha)
	}
	if o := PorSitios.Valida(); o != PorSitios {
		t.Errorf("Valida() se comió un orden bueno: %q", o)
	}
}

func TestLaPortadaVaciaNoRevienta(t *testing.T) {
	r := Nuevo(callado(), nil)
	if filas := r.Portada(PorFecha); len(filas) != 0 {
		t.Errorf("salieron %d filas de la nada", len(filas))
	}
}

func titulosDe(fs []Fila) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Titulo)
	}
	return out
}
