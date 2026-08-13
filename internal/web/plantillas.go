package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/davic80/iman"
)

// paginas son las plantillas de contenido. Cada una se compila junto a base.html
// en su propio conjunto: si se parsearan todas a la vez, el segundo
// {{define "contenido"}} pisaria al primero y la ultima pagina ganaria siempre.
var paginas = []string{"buscar", "novedades", "salud"}

type juegoPlantillas map[string]*template.Template

func compilarPlantillas(archivos fs.FS) (juegoPlantillas, error) {
	juego := make(juegoPlantillas, len(paginas))
	for _, p := range paginas {
		// filtros.html y cartel.html van con todas: son trozos sueltos que
		// buscar y novedades pintan igual, y compilarlos de más en salud no
		// molesta a nadie.
		t, err := template.New("base.html").ParseFS(archivos,
			"plantillas/base.html",
			"plantillas/filtros.html",
			"plantillas/cartel.html",
			"plantillas/"+p+".html",
		)
		if err != nil {
			return nil, fmt.Errorf("compilando plantilla %q: %w", p, err)
		}
		juego[p] = t
	}
	return juego, nil
}

// pintar escribe una plantilla en la respuesta.
//
// Se renderiza primero a un buffer y solo despues se copia al ResponseWriter.
// Asi, si la plantilla falla a mitad, todavia estamos a tiempo de devolver un
// 500 limpio en lugar de un 200 con media pagina escrita.
func (s *Servidor) pintar(w http.ResponseWriter, r *http.Request, pagina string, datos any) {
	t, ok := s.plantillas[pagina]
	if !ok {
		s.log.Error("plantilla desconocida", "pagina", pagina)
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", datos); err != nil {
		s.log.Error("pintando plantilla", "pagina", pagina, "error", err)
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		s.log.Warn("escribiendo respuesta", "ruta", r.URL.Path, "error", err)
	}
}

// ficherosEstaticos sirve /estaticos/ desde el binario.
func ficherosEstaticos() http.Handler {
	sub, err := fs.Sub(iman.Estaticos, "estaticos")
	if err != nil {
		// Solo puede pasar si el //go:embed no encontro el directorio, y eso lo
		// detecta el compilador. Si llega aqui, el binario esta mal construido.
		panic(fmt.Sprintf("recursos estaticos empotrados mal: %v", err))
	}
	return http.StripPrefix("/estaticos/", http.FileServer(http.FS(sub)))
}
