package dominios

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/davic80/iman/internal/conectores"
)

func TestElEstadoSobreviveAlReinicio(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "estado.json")

	uno, err := CargarEstado(ruta)
	if err != nil {
		t.Fatalf("CargarEstado: %v", err)
	}
	if err := uno.Guardar("Sitio", "https://sitio.nuevo"); err != nil {
		t.Fatalf("Guardar: %v", err)
	}

	dos, err := CargarEstado(ruta)
	if err != nil {
		t.Fatalf("recargando: %v", err)
	}
	if got, ok := dos.Dominio("Sitio"); !ok || got != "https://sitio.nuevo" {
		t.Errorf("Dominio = %q (%v), quiero el que se guardó", got, ok)
	}
}

func TestNoHaberEstadoNoEsUnError(t *testing.T) {
	e, err := CargarEstado(filepath.Join(t.TempDir(), "no-existe.json"))
	if err != nil {
		t.Fatalf("un fichero que no existe es el primer arranque, no un error: %v", err)
	}
	if _, ok := e.Dominio("Sitio"); ok {
		t.Error("no debería saber nada de ningún sitio")
	}
}

// Es un caché, no una base de datos: un JSON a medio escribir no puede impedir
// que Imán arranque. Se avisa y se empieza de cero.
func TestUnEstadoCorruptoNoImpideArrancar(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "estado.json")
	if err := os.WriteFile(ruta, []byte(`{"Sitio": {"domin`), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := CargarEstado(ruta)
	if err == nil {
		t.Error("debería avisar de que el fichero no se entiende")
	}
	if e == nil {
		t.Fatal("aun así tiene que devolver un estado usable")
	}
	if _, ok := e.Dominio("Sitio"); ok {
		t.Error("no debería haberse quedado con nada del fichero roto")
	}
	// Y tiene que poder seguir usándose.
	if err := e.Guardar("Sitio", "https://sitio.uno"); err != nil {
		t.Errorf("Guardar tras un fichero corrupto: %v", err)
	}
}

// Escribir tiene que ser atómico: si se corta a medias, lo que había antes
// sigue estando. Se comprueba que no queda ningún temporal suelto.
func TestGuardarNoDejaBasura(t *testing.T) {
	dir := t.TempDir()
	e, err := CargarEstado(filepath.Join(dir, "estado.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"https://uno", "https://dos", "https://tres"} {
		if err := e.Guardar("Sitio", d); err != nil {
			t.Fatal(err)
		}
	}

	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 1 || entradas[0].Name() != "estado.json" {
		var nombres []string
		for _, e := range entradas {
			nombres = append(nombres, e.Name())
		}
		t.Errorf("en el directorio hay %v, quiero solo estado.json", nombres)
	}
}

// Al arrancar se vuelve al último dominio bueno sin pedir nada por la red: un
// despliegue no puede costar una ronda de descubrimiento.
func TestRestaurarNoTocaLaRed(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "estado.json")
	estado, err := CargarEstado(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if err := estado.Guardar("Sitio", "https://sitio.guardado"); err != nil {
		t.Fatal(err)
	}

	sitio := &sitioFalso{nombre: "Sitio", base: "https://sitio.de-fabrica"}
	r := Nuevo(&clienteFalso{}, mudo(), estado, sitio)
	r.Restaurar()

	if got := sitio.Base(); got != "https://sitio.guardado" {
		t.Errorf("base = %q, quiero la guardada", got)
	}
	if p := sitio.preguntados(); len(p) != 0 {
		t.Errorf("se sondearon %v; restaurar no debería tocar la red", p)
	}
}

// Sin estado el resolutor funciona igual, solo que olvidando. No puede ser un
// requisito tener volumen para poder arrancar.
func TestSinEstadoFuncionaIgual(t *testing.T) {
	sitio := &sitioFalso{
		nombre: "Sitio", base: "https://sitio.uno",
		paginas: map[string]conectores.Sondeo{"https://sitio.uno": buena(14)},
	}
	r := Nuevo(&clienteFalso{}, mudo(), nil, sitio)
	r.Restaurar()

	if err := r.Revisar1(context.Background(), sitio); err != nil {
		t.Errorf("Revisar1 sin estado: %v", err)
	}
}
