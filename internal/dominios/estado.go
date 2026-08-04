package dominios

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Estado recuerda entre arranques en qué dominio estaba cada sitio.
//
// Sin esto, cada despliegue empezaría de cero y tendría que redescubrirlo todo;
// con veinte despliegues en una tarde, eso es pedirle a los sitios muchas más
// peticiones de las necesarias. Perderlo no rompe nada: solo cuesta una ronda.
type Estado struct {
	ruta string

	mu     sync.Mutex
	sitios map[string]sitioGuardado
}

type sitioGuardado struct {
	Dominio string    `json:"dominio"`
	Visto   time.Time `json:"visto"`
}

// CargarEstado lee el fichero. Un fichero que no existe no es un error: es el
// primer arranque.
//
// Uno corrupto tampoco lo es. Es un caché, no una base de datos: si no se
// entiende se tira y se vuelve a descubrir, que es mucho mejor que negarse a
// arrancar por un JSON a medio escribir.
func CargarEstado(ruta string) (*Estado, error) {
	e := &Estado{ruta: ruta, sitios: map[string]sitioGuardado{}}
	if ruta == "" {
		return e, nil
	}

	crudo, err := os.ReadFile(ruta)
	if os.IsNotExist(err) {
		return e, nil
	}
	if err != nil {
		return e, fmt.Errorf("leyendo %s: %w", ruta, err)
	}
	if err := json.Unmarshal(crudo, &e.sitios); err != nil {
		e.sitios = map[string]sitioGuardado{}
		return e, fmt.Errorf("%s no se entiende, se empieza de cero: %w", ruta, err)
	}
	return e, nil
}

// Dominio devuelve el último dominio bueno conocido de un sitio.
func (e *Estado) Dominio(sitio string) (string, bool) {
	if e == nil {
		return "", false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.sitios[sitio]
	return s.Dominio, ok && s.Dominio != ""
}

// Guardar apunta el dominio bueno de un sitio y escribe el fichero.
func (e *Estado) Guardar(sitio, dominio string) error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if s, ok := e.sitios[sitio]; ok && s.Dominio == dominio {
		// Sin cambios, pero sí interesa cuándo se comprobó por última vez.
		s.Visto = time.Now().UTC()
		e.sitios[sitio] = s
	} else {
		e.sitios[sitio] = sitioGuardado{Dominio: dominio, Visto: time.Now().UTC()}
	}
	return e.escribir()
}

// escribir vuelca el fichero entero. Hay que llamarlo con el candado cogido.
//
// Se escribe a un temporal y se renombra: un rename es atómico, así que un
// corte de corriente a media escritura deja el fichero anterior intacto en vez
// de uno a medias que no se entiende.
func (e *Estado) escribir() error {
	if e.ruta == "" {
		return nil
	}
	crudo, err := json.MarshalIndent(e.sitios, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(e.ruta), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(e.ruta), ".estado-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // No hace nada si el rename salió bien

	if _, err := tmp.Write(crudo); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), e.ruta)
}
