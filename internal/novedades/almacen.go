package novedades

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// formatoActual es la versión del fichero guardado.
//
// Va escrita dentro porque lo que se guarda son structs de Go serializados tal
// cual, y alguno lleva enteros con significado: titulos.Idioma es un iota, así
// que el día que se añada un idioma en medio de la lista los apuntes viejos
// dirían otra cosa. Subiendo el número se tira lo anterior y se vuelve a mirar,
// que cuesta una ronda.
const formatoActual = 1

// Almacen guarda los apuntes en disco para que la portada no salga vacía
// después de cada despliegue.
//
// Es un caché, no una base de datos: si el fichero no está o no se entiende, se
// empieza de cero y a la siguiente ronda ya hay novedades. Por eso nada de lo
// que pasa aquí puede impedir que Imán arranque.
type Almacen struct {
	ruta string
	mu   sync.Mutex
}

// NuevoAlmacen apunta a un fichero. Con la ruta vacía no se guarda nada, que es
// lo que hacen los tests y lo que pasa si alguien arranca sin volumen.
func NuevoAlmacen(ruta string) *Almacen { return &Almacen{ruta: ruta} }

type fichero struct {
	Formato int      `json:"formato"`
	Apuntes []Apunte `json:"apuntes"`
}

// Cargar lee los apuntes guardados. Que no haya fichero no es un error: es el
// primer arranque.
func (a *Almacen) Cargar() ([]Apunte, error) {
	if a == nil || a.ruta == "" {
		return nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	crudo, err := os.ReadFile(a.ruta)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leyendo %s: %w", a.ruta, err)
	}

	var f fichero
	if err := json.Unmarshal(crudo, &f); err != nil {
		return nil, fmt.Errorf("%s no se entiende, se empieza de cero: %w", a.ruta, err)
	}
	if f.Formato != formatoActual {
		return nil, fmt.Errorf("%s está en el formato %d y ahora se usa el %d, se empieza de cero",
			a.ruta, f.Formato, formatoActual)
	}
	return f.Apuntes, nil
}

// Guardar vuelca los apuntes al fichero.
//
// Se escribe a un temporal y se renombra, igual que el estado de los dominios:
// un rename es atómico, así que un corte a media escritura deja el fichero
// anterior entero en vez de uno a medias que ya no se entiende.
func (a *Almacen) Guardar(apuntes []Apunte) error {
	if a == nil || a.ruta == "" {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	crudo, err := json.MarshalIndent(fichero{Formato: formatoActual, Apuntes: apuntes}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.ruta), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(a.ruta), ".novedades-*.json")
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
	return os.Rename(tmp.Name(), a.ruta)
}
