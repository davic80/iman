package web

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/tmdb"
)

// tiempoCarteles es lo que se espera a TMDB antes de pintar la página sin
// carátulas. La carátula es adorno: hacer esperar por ella sería ponerla por
// delante de los resultados, que es a lo que se viene. Lo que no llegue a
// tiempo estará en la caché para la próxima.
const tiempoCarteles = 4 * time.Second

// aLaVezTMDB son las preguntas simultáneas a TMDB. Su límite público es muy
// alto, pero abrir cuarenta conexiones de golpe para decorar una lista tampoco
// hace falta.
const aLaVezTMDB = 6

// fichas pregunta a TMDB por todas las filas a la vez y devuelve lo que haya
// llegado, en el mismo orden. Las que no tengan ficha —o no lleguen a tiempo—
// vuelven vacías, y la fila se pinta como se pintaba antes de existir TMDB.
func (s *Servidor) fichas(ctx context.Context, filas []buscador.Resultado) []tmdb.Ficha {
	fichas := make([]tmdb.Ficha, len(filas))
	if !s.carteles.Activo() || len(filas) == 0 {
		return fichas
	}

	ctx, cancelar := context.WithTimeout(ctx, tiempoCarteles)
	defer cancelar()

	var grupo sync.WaitGroup
	hueco := make(chan struct{}, aLaVezTMDB)

	for i, fila := range filas {
		grupo.Add(1)
		go func() {
			defer grupo.Done()

			hueco <- struct{}{}
			defer func() { <-hueco }()

			if f, hay := s.carteles.Buscar(ctx, fila.Titulo, fila.Info); hay {
				fichas[i] = f
			}
		}()
	}
	grupo.Wait()

	return fichas
}

// cartel sirve una carátula de TMDB desde aquí.
//
// La trae el servidor y no el navegador por lo mismo que el .torrent: esta
// instancia es privada, y una página privada que va cargando imágenes de fuera
// le cuenta a un tercero qué se está mirando.
func (s *Servidor) cartel(w http.ResponseWriter, r *http.Request) {
	tamaño, fichero := r.PathValue("tamaño"), r.PathValue("fichero")

	cuerpo, tipo, err := s.carteles.Cartel(r.Context(), tamaño, "/"+fichero)
	if err != nil {
		s.log.Warn("no se pudo traer una carátula", "tamaño", tamaño, "fichero", fichero, "error", err)
		http.NotFound(w, r)
		return
	}
	defer cuerpo.Close()

	w.Header().Set("Content-Type", tipo)
	// El nombre del fichero en TMDB es el resumen de la propia imagen: si
	// cambia la carátula, cambia la URL. Así que esta se puede guardar.
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")

	if _, err := io.Copy(w, cuerpo); err != nil {
		s.log.Warn("se cortó la carátula", "fichero", fichero, "error", err)
	}
}
