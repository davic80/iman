package web

import (
	"os"
	"strconv"
	"time"

	"github.com/davic80/iman/internal/novedades"
)

// Config recoge todo lo que se puede tocar sin recompilar. Los valores por
// defecto tienen que ser los correctos para produccion: si alguien despliega
// sin un .env, la app debe arrancar y comportarse bien igualmente.
type Config struct {
	// Addr es la direccion de escucha. Dentro del contenedor siempre es :8080;
	// solo Caddy habla con nosotros, nunca internet directamente.
	Addr string

	// Version es el SHA del commit con el que se construyo la imagen. Lo
	// inyecta el Dockerfile via -ldflags y se muestra en /salud para saber
	// exactamente que hay corriendo en el servidor.
	Version string

	// EstadoPath es donde se persiste que dominio funciona para cada sitio.
	// Vive en un volumen para sobrevivir a los despliegues: perderlo significa
	// volver a resolver todos los dominios desde cero en el primer arranque.
	EstadoPath string

	// Presupuesto de tiempo de una busqueda completa. Lo que no llegue a
	// tiempo, no llega: se muestran los resultados parciales.
	TiempoBusqueda time.Duration

	// NovedadesPath es donde se guarda la portada de novedades. Va al lado del
	// estado y por el mismo motivo: sin el, cada despliegue dejaria la portada
	// vacia hasta la primera ronda.
	NovedadesPath string

	// RondaNovedades es cada cuanto se mira que han subido los sitios.
	RondaNovedades time.Duration

	// TMDB es la credencial para pedir carátulas y fichas. Es el unico valor
	// secreto de toda la configuracion: vive en el .env, que no se versiona, y
	// no aparece en .env.example. Vacia significa "sin carátulas", que es un
	// modo de funcionamiento normal y no un error.
	TMDB string
}

// CargarConfig lee la configuracion del entorno.
//
// versionCompilada es la que inyecta el Dockerfile con -ldflags. La variable de
// entorno puede pisarla, que resulta comodo al depurar en local.
func CargarConfig(versionCompilada string) Config {
	return Config{
		Addr:           env("IMAN_ADDR", ":8080"),
		Version:        env("IMAN_VERSION", versionCompilada),
		EstadoPath:     env("IMAN_ESTADO", "/datos/estado.json"),
		TiempoBusqueda: envDuracion("IMAN_TIEMPO_BUSQUEDA", 20*time.Second),
		NovedadesPath:  env("IMAN_NOVEDADES", "/datos/novedades.json"),
		RondaNovedades: envDuracion("IMAN_RONDA_NOVEDADES", novedades.CadaPorDefecto),
		TMDB:           env("IMAN_TMDB", ""),
	}
}

func env(clave, porDefecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return porDefecto
}

func envDuracion(clave string, porDefecto time.Duration) time.Duration {
	v := os.Getenv(clave)
	if v == "" {
		return porDefecto
	}
	// Se aceptan las dos formas: "20s" y "20". Un numero suelto son segundos,
	// que es lo que la gente escribe cuando no se acuerda del formato de Go.
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return porDefecto
}
