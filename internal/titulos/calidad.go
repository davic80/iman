package titulos

import "regexp"

// Calidad es la calidad de vídeo, ya normalizada. Los sitios escriben lo mismo
// de muchas formas ("BR-Rip", "brrip", "BRrip") y aquí todas acaban igual.
type Calidad string

const (
	CalidadDesconocida Calidad = ""

	Cal4K       Calidad = "4K"
	Cal1080p    Calidad = "1080p"
	Cal720p     Calidad = "720p"
	CalMicroHD  Calidad = "MicroHD"
	CalBluRay   Calidad = "BluRay"
	CalWebDL    Calidad = "WEB-DL"
	CalBRRip    Calidad = "BRRip"
	CalHDRip    Calidad = "HDRip"
	CalHDTV     Calidad = "HDTV"
	CalDVDRip   Calidad = "DVDRip"
	CalScreener Calidad = "Screener"
	CalTS       Calidad = "TS"
	CalCAM      Calidad = "CAM"
)

// rangos ordena las calidades de mejor a peor para poder poner delante lo que
// se ve bien. El número solo sirve para comparar entre sí.
//
// Una grabación de cine mala es mala aunque la hayan subido a 1080p, así que
// las fuentes de sala (CAM, TS, screener) quedan por debajo de todo lo demás.
var rangos = map[Calidad]int{
	Cal4K:       100,
	Cal1080p:    90,
	CalBluRay:   85,
	CalMicroHD:  80,
	CalWebDL:    75,
	Cal720p:     70,
	CalBRRip:    60,
	CalHDRip:    55,
	CalHDTV:     50,
	CalDVDRip:   40,
	CalScreener: 20,
	CalTS:       10,
	CalCAM:      0,
}

// Rango sirve para ordenar resultados. Lo desconocido va por debajo de
// cualquier calidad reconocida, pero por encima de una grabación de sala:
// no saber es mejor que saber que es mala.
func (c Calidad) Rango() int {
	if r, ok := rangos[c]; ok {
		return r
	}
	return 30
}

// El orden importa: las fuentes de sala se comprueban antes que la resolución
// para que un "HQ-TS 1080p" acabe siendo TS y no 1080p.
var marcasCalidad = []struct {
	re      *regexp.Regexp
	calidad Calidad
}{
	{regexp.MustCompile(`\b(cam ?rip|cam)\b`), CalCAM},
	{regexp.MustCompile(`\b(hq ?ts|hdts|ts ?rip|telesync|ts|tc|telecine)\b`), CalTS},
	{regexp.MustCompile(`\b(screener|dvdscr|scr)\b`), CalScreener},
	{regexp.MustCompile(`\b(4k|2160p|uhd)\b`), Cal4K},
	{regexp.MustCompile(`\b1080p?\b`), Cal1080p},
	{regexp.MustCompile(`\bmicrohd\b`), CalMicroHD},
	{regexp.MustCompile(`\b(bluray|blu ray|bdrip|bd ?remux|remux)\b`), CalBluRay},
	{regexp.MustCompile(`\b(web ?dl|webrip|web)\b`), CalWebDL},
	{regexp.MustCompile(`\b720p?\b`), Cal720p},
	{regexp.MustCompile(`\b(brrip|br ?rip|bdr)\b`), CalBRRip},
	{regexp.MustCompile(`\b(hdrip|hd ?rip)\b`), CalHDRip},
	{regexp.MustCompile(`\b(hdtv|tvrip|pdtv)\b`), CalHDTV},
	{regexp.MustCompile(`\b(dvdrip|dvd ?rip|dvdr|dvd)\b`), CalDVDRip},
}

// DetectarCalidad saca la calidad de un texto libre. Devuelve
// CalidadDesconocida si no encuentra ninguna marca, incluido el "---" que usa
// EliteTorrent cuando no la sabe.
func DetectarCalidad(s string) Calidad {
	n := normalizar(s)
	for _, m := range marcasCalidad {
		if m.re.MatchString(n) {
			return m.calidad
		}
	}
	return CalidadDesconocida
}
