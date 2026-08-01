package conectores

import "testing"

func TestParsearTamaño(t *testing.T) {
	const (
		kb = 1 << 10
		mb = 1 << 20
		gb = 1 << 30
	)
	casos := []struct {
		texto  string
		quiero int64
	}{
		// Las tres formas que conviven en una misma página de EliteTorrent.
		{"2.21GB", gigas(2.21)},
		{"2 GBs", 2 * gb},
		{"700 MBs", 700 * mb},

		{"1,5 GB", gigas(1.5)}, // Coma decimal, a la española
		{"512 KB", 512 * kb},
		{"4 TB", 4 * 1 << 40},
		{"900 bytes", 900},
		{"1.93gb", gigas(1.93)},
		{"Tamaño: 2.21GB", gigas(2.21)},

		{"", 0},
		{"---", 0},
		{"desconocido", 0},
		{"GB", 0},
	}
	for _, c := range casos {
		if got := ParsearTamaño(c.texto); got != c.quiero {
			t.Errorf("ParsearTamaño(%q) = %d, quiero %d", c.texto, got, c.quiero)
		}
	}
}

func TestFormatearTamaño(t *testing.T) {
	casos := []struct {
		bytes  int64
		quiero string
	}{
		{0, ""},  // No se sabe: mejor no enseñar nada que un "0 B" falso
		{-1, ""}, //
		{512, "512 B"},
		{2 * 1 << 30, "2.0 GB"},
		{gigas(2.21), "2.2 GB"},
		{700 * 1 << 20, "700.0 MB"},
	}
	for _, c := range casos {
		if got := FormatearTamaño(c.bytes); got != c.quiero {
			t.Errorf("FormatearTamaño(%d) = %q, quiero %q", c.bytes, got, c.quiero)
		}
	}
}

// Lo que entra tiene que poder volver a salir parecido, que es lo que ve el
// usuario en la lista de resultados.
func TestTamañoIdaYVuelta(t *testing.T) {
	for _, texto := range []string{"2 GBs", "700 MBs", "512 KB"} {
		b := ParsearTamaño(texto)
		if b == 0 {
			t.Fatalf("ParsearTamaño(%q) = 0", texto)
		}
		if vuelta := ParsearTamaño(FormatearTamaño(b)); vuelta != b {
			t.Errorf("%q: ida %d, vuelta %d", texto, b, vuelta)
		}
	}
}
