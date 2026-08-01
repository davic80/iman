package conectores

import (
	"regexp"
	"strconv"
	"strings"
)

// Los sitios escriben el tamaño de todas las formas imaginables. En una sola
// página de EliteTorrent conviven "2.21GB", "2 GBs" y "700 MBs", así que el
// espacio es opcional y la "s" del plural también.
var reTamaño = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(bytes?|[kmgt]i?bs?|b)\b`)

var unidades = map[string]int64{
	"b": 1,
	"k": 1 << 10,
	"m": 1 << 20,
	"g": 1 << 30,
	"t": 1 << 40,
}

// ParsearTamaño convierte a bytes lo que ponga el sitio. Devuelve 0 si no
// entiende nada, que es como se representa "no se sabe".
//
// Se usa base 1024 porque es lo que usan los clientes de torrent, que es de
// donde los sitios copian la cifra.
func ParsearTamaño(s string) int64 {
	m := reTamaño.FindStringSubmatch(s)
	if m == nil {
		return 0
	}

	n, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
	if err != nil || n < 0 {
		return 0
	}

	unidad := strings.ToLower(m[2])[:1]
	factor, ok := unidades[unidad]
	if !ok {
		return 0
	}
	return int64(n * float64(factor))
}

// FormatearTamaño escribe un tamaño en bytes como lo escribiría una persona.
// Cadena vacía si no se sabe, para que la interfaz no enseñe un "0 B" falso.
func FormatearTamaño(b int64) string {
	if b <= 0 {
		return ""
	}
	const k = 1024
	if b < k {
		return strconv.FormatInt(b, 10) + " B"
	}
	div, exp := int64(k), 0
	for n := b / k; n >= k && exp < 3; n /= k {
		div *= k
		exp++
	}
	return strconv.FormatFloat(float64(b)/float64(div), 'f', 1, 64) +
		" " + [...]string{"KB", "MB", "GB", "TB"}[exp]
}
