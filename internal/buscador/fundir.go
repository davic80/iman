package buscador

import (
	"strconv"
	"strings"

	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
)

// Resultado es una fila de la lista: un torrent y todos los sitios donde se ha
// encontrado.
//
// El que manda es el mejor de ellos —el que da más facilidades para llegar al
// contenido—, y los demás quedan apuntados detrás. No se tira ninguno: fundir
// aquí es juntar en una fila, nunca esconder. Si la fusión se equivoca y eran
// dos cosas distintas, el usuario sigue teniendo delante el enlace de los dos
// sitios.
type Resultado struct {
	conectores.Resultado
	Repetidos []conectores.Resultado
}

// Sitios devuelve todos los sitios que tienen esto, empezando por el que manda.
func (r Resultado) Sitios() []string {
	out := make([]string, 0, len(r.Repetidos)+1)
	out = append(out, r.Sitio)
	for _, o := range r.Repetidos {
		out = append(out, o.Sitio)
	}
	return out
}

// Fundir junta en una sola fila los resultados que son el mismo torrent.
//
// Es público porque la portada de novedades agrupa lo mismo: si allí se
// juntaran las películas con otro criterio, los mismos datos saldrían agrupados
// de dos maneras distintas según por qué página se entre.
//
// Hay dos formas de decirlo, y son muy distintas de fiar:
//
//   - El infohash. Es certeza: identifica al fichero, lo escriba como lo
//     escriba cada sitio. Lo malo es que casi nunca se sabe todavía, porque
//     vive en el magnet y los magnets salen de la ficha, que solo se pide
//     cuando el usuario pincha.
//   - El parecido: misma obra, mismo año, mismo capítulo y misma calidad. Eso
//     es una apuesta, no una certeza, así que solo se hace entre sitios
//     distintos y sin contradecir los tamaños. Dos entradas del mismo sitio no
//     se tocan: si el sitio las publica por separado, él sabrá por qué.
func Fundir(rs []conectores.Resultado) []Resultado {
	var out []Resultado

	// Primera vuelta: lo que se sabe seguro. El infohash cuando lo hay, y si no
	// la propia URL, que caza lo que un sitio repite en su propia lista.
	porClave := make(map[string]int, len(rs))
	for _, r := range rs {
		clave := r.InfoHash()
		if clave == "" {
			clave = r.Sitio + "\x00" + r.Ficha
		}
		if i, ok := porClave[clave]; ok {
			out[i] = juntar(out[i], r)
			continue
		}
		porClave[clave] = len(out)
		out = append(out, Resultado{Resultado: r})
	}

	// Segunda vuelta: los que solo se parecen. Se recorre el grupo entero
	// porque una fila puede no admitir a este resultado (mismo sitio, tamaños
	// que no cuadran) y la siguiente sí.
	final := make([]Resultado, 0, len(out))
	grupos := make(map[string][]int, len(out))
	for _, r := range out {
		clave := claveDeParecido(r.Resultado)
		if clave != "" {
			if i, ok := hueco(final, grupos[clave], r.Resultado); ok {
				final[i] = juntar(final[i], r.Resultado)
				final[i].Repetidos = append(final[i].Repetidos, r.Repetidos...)
				continue
			}
			grupos[clave] = append(grupos[clave], len(final))
		}
		final = append(final, r)
	}
	return final
}

// hueco busca en un grupo de filas parecidas una que admita a r.
func hueco(final []Resultado, grupo []int, r conectores.Resultado) (int, bool) {
	for _, i := range grupo {
		if cabe(final[i], r) {
			return i, true
		}
	}
	return 0, false
}

// cabe dice si r puede entrar en una fila que ya existe.
func cabe(fila Resultado, r conectores.Resultado) bool {
	for _, otro := range append([]conectores.Resultado{fila.Resultado}, fila.Repetidos...) {
		if otro.Sitio == r.Sitio {
			return false
		}
		if !tamañosCompatibles(otro.Tamaño, r.Tamaño) {
			return false
		}
	}
	return true
}

// tamañosCompatibles compara dos pesos sin exigir que sean idénticos.
//
// El mismo fichero pesa lo mismo, pero cada sitio lo escribe redondeado a su
// manera ("2.0 GB" contra "2.05 GB"), así que se admite un margen. Un tamaño
// desconocido no contradice a nadie: no sabemos, y no saber no es discrepar.
func tamañosCompatibles(a, b int64) bool {
	if a <= 0 || b <= 0 {
		return true
	}
	if a > b {
		a, b = b, a
	}
	return b-a <= b/20 // 5%
}

// juntar mete r en la fila y deja delante al que más facilidades da.
//
// No se copian datos de uno a otro. Aunque en la fusión por infohash sería
// seguro (es el mismo fichero, luego el mismo peso), en la fusión por parecido
// sería inventarse el dato de una fila que a lo mejor no es la misma cosa, y
// mejor una sola regla que dos.
func juntar(fila Resultado, r conectores.Resultado) Resultado {
	if mejor(r, fila.Resultado) {
		fila.Repetidos = append(fila.Repetidos, fila.Resultado)
		fila.Resultado = r
		return fila
	}
	fila.Repetidos = append(fila.Repetidos, r)
	return fila
}

// mejor dice si a merece mandar sobre b.
//
// Manda quien deja al usuario más cerca del contenido: un magnet se abre en el
// cliente de torrents de un clic, un .torrent hay que traerlo, y una ficha
// pelada obliga a resolverla primero. Después, quien traiga más datos para
// enseñar. El último desempate es el nombre del sitio, y está para que la lista
// no baile entre dos recargas iguales.
func mejor(a, b conectores.Resultado) bool {
	if pa, pb := puntosDeEnlace(a), puntosDeEnlace(b); pa != pb {
		return pa > pb
	}
	if a.SemillasConocidas() != b.SemillasConocidas() {
		return a.SemillasConocidas()
	}
	if a.SemillasConocidas() && a.Semillas != b.Semillas {
		return a.Semillas > b.Semillas
	}
	if (a.Tamaño > 0) != (b.Tamaño > 0) {
		return a.Tamaño > 0
	}
	return a.Sitio < b.Sitio
}

func puntosDeEnlace(r conectores.Resultado) int {
	switch {
	case r.Magnet != "":
		return 2
	case r.Torrent != "":
		return 1
	default:
		return 0
	}
}

// claveDeParecido describe lo que tendrían en común dos publicaciones del mismo
// torrent en sitios distintos. Vacía cuando no hay obra: sin título limpio no
// se puede afirmar nada, y el año o la calidad por sí solos no distinguen nada.
func claveDeParecido(r conectores.Resultado) string {
	obra := titulos.SinArticulo(r.Info.Obra)
	if obra == "" {
		return ""
	}
	return strings.Join([]string{
		obra,
		strconv.Itoa(r.Info.Año),
		strconv.Itoa(r.Info.Temporada),
		strconv.Itoa(r.Info.Episodio),
		string(r.Info.Calidad),
	}, "\x00")
}
