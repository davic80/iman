//go:build vivo

// Estos tests salen a internet de verdad. No los compila nadie salvo que se
// pidan a mano:
//
//	go test -tags vivo -v ./internal/conectores/
//
// Van aparte porque el CI no puede depender de que un sitio esté vivo, pero
// hacen falta: son la forma de enterarse de que un sitio ha cambiado el HTML y
// las fixtures se han quedado viejas. Cuando fallen, hay que recapturarlas.
package conectores

import (
	"context"
	"testing"
	"time"

	"github.com/davic80/iman/internal/titulos"
)

func TestVivoEliteTorrent(t *testing.T) {
	ctx, cancelar := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelar()

	e := NuevoEliteTorrent(nil)

	// Una consulta distinta a la de las fixtures, para no probar el parser
	// contra la única página que ya sabemos que entiende.
	rs, err := e.Buscar(ctx, "el padrino")
	if err != nil {
		t.Fatalf("Buscar: %v", err)
	}
	if len(rs) == 0 {
		t.Fatal("cero resultados: o el sitio ha cambiado el HTML o ha cambiado de dominio")
	}
	t.Logf("%d resultados", len(rs))

	cuenta := map[titulos.Idioma]int{}
	for _, r := range rs {
		cuenta[r.Info.Idioma]++
		if r.Titulo == "" || r.Ficha == "" {
			t.Errorf("resultado incompleto: %+v", r)
		}
	}
	t.Logf("idiomas: %v", cuenta)
	if cuenta[titulos.Desconocido] == len(rs) {
		t.Error("ningún resultado trae idioma; la etiqueta del sitio ha cambiado")
	}

	// Y que del primero en castellano se puede sacar de verdad un magnet, que
	// es lo único que el usuario se lleva a casa.
	for _, r := range rs {
		if r.Info.Idioma.Veredicto() != titulos.Acepta {
			continue
		}
		if err := e.Resolver(ctx, &r); err != nil {
			t.Fatalf("Resolver %q: %v", r.Titulo, err)
		}
		t.Logf("%q -> %.60s", r.Titulo, r.Enlace())
		if r.Magnet == "" && r.Torrent == "" {
			t.Errorf("%q no dio ni magnet ni .torrent", r.Titulo)
		}
		return
	}
	t.Error("ningún resultado en castellano para 'el padrino'; sospechoso")
}
