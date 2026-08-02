package buscador

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
)

func mudo() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// falso es un conector de mentira: devuelve lo que se le diga, tarda lo que se
// le diga y falla cuando se le diga.
type falso struct {
	nombre    string
	resultado []conectores.Resultado
	err       error
	tarda     time.Duration
	revienta  bool
	llamadas  int
}

func (f *falso) Nombre() string { return f.nombre }

func (f *falso) Buscar(ctx context.Context, consulta string) ([]conectores.Resultado, error) {
	f.llamadas++
	if f.revienta {
		panic("el HTML ha cambiado y nadie lo vio venir")
	}
	if f.tarda > 0 {
		select {
		case <-time.After(f.tarda):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.resultado, f.err
}

func res(sitio, titulo string, idioma titulos.Idioma, cal titulos.Calidad) conectores.Resultado {
	return conectores.Resultado{
		Sitio:    sitio,
		Titulo:   titulo,
		Ficha:    "https://" + sitio + "/" + titulo,
		Semillas: -1,
		Clientes: -1,
		Info:     titulos.Info{Obra: titulo, Idioma: idioma, Calidad: cal},
	}
}

func TestBuscarJuntaVariosSitios(t *testing.T) {
	uno := &falso{nombre: "Uno", resultado: []conectores.Resultado{
		res("Uno", "peli a", titulos.Castellano, titulos.Cal720p),
	}}
	dos := &falso{nombre: "Dos", resultado: []conectores.Resultado{
		res("Dos", "peli b", titulos.Castellano, titulos.Cal1080p),
	}}

	b := Nuevo(mudo(), time.Second, uno, dos)
	got := b.Buscar(context.Background(), "peli", Opciones{})

	if len(got.Resultados) != 2 {
		t.Fatalf("%d resultados, quiero 2", len(got.Resultados))
	}
	if !got.Completa() {
		t.Errorf("la búsqueda debería estar completa, fallos: %v", got.Fallos)
	}
	// El de más calidad va primero.
	if got.Resultados[0].Titulo != "peli b" {
		t.Errorf("primero = %q, quiero el 1080p", got.Resultados[0].Titulo)
	}
}

// Lo importante del motor: que un sitio roto no se lleve por delante a los
// demás. Si esto falla, una búsqueda vale lo que valga el peor de los sitios.
func TestUnSitioCaidoNoEstropeaElResto(t *testing.T) {
	bueno := &falso{nombre: "Bueno", resultado: []conectores.Resultado{
		res("Bueno", "peli", titulos.Castellano, titulos.Cal1080p),
	}}
	roto := &falso{nombre: "Roto", err: errors.New("503")}

	b := Nuevo(mudo(), time.Second, bueno, roto)
	got := b.Buscar(context.Background(), "peli", Opciones{})

	if len(got.Resultados) != 1 {
		t.Fatalf("%d resultados, quiero 1", len(got.Resultados))
	}
	if len(got.Fallos) != 1 || got.Fallos[0].Sitio != "Roto" {
		t.Fatalf("Fallos = %+v, quiero uno de Roto", got.Fallos)
	}
	if got.Completa() {
		t.Error("con un sitio caído la búsqueda no está completa")
	}
}

// Un parser que revienta es cuestión de tiempo: viven de adivinar HTML ajeno.
// El proceso tiene que seguir en pie y el resto de sitios contestar igual.
func TestUnConectorQueRevientaNoTiraElProceso(t *testing.T) {
	bueno := &falso{nombre: "Bueno", resultado: []conectores.Resultado{
		res("Bueno", "peli", titulos.Castellano, titulos.Cal720p),
	}}
	kamikaze := &falso{nombre: "Kamikaze", revienta: true}

	b := Nuevo(mudo(), time.Second, bueno, kamikaze)
	got := b.Buscar(context.Background(), "peli", Opciones{})

	if len(got.Resultados) != 1 {
		t.Fatalf("%d resultados, quiero 1", len(got.Resultados))
	}
	if len(got.Fallos) != 1 || got.Fallos[0].Sitio != "Kamikaze" {
		t.Fatalf("Fallos = %+v", got.Fallos)
	}
	if s := saludDe(t, b, "Kamikaze"); s.Fallos != 1 {
		t.Errorf("el panic debería contar como fallo, Fallos = %d", s.Fallos)
	}
}

// El presupuesto es del usuario, no de los sitios: pasado el plazo se contesta
// con lo que haya llegado.
func TestElSitioLentoNoHaceEsperarALosDemas(t *testing.T) {
	rapido := &falso{nombre: "Rapido", resultado: []conectores.Resultado{
		res("Rapido", "peli", titulos.Castellano, titulos.Cal720p),
	}}
	lento := &falso{nombre: "Lento", tarda: 10 * time.Second, resultado: []conectores.Resultado{
		res("Lento", "otra", titulos.Castellano, titulos.Cal1080p),
	}}

	b := Nuevo(mudo(), 100*time.Millisecond, rapido, lento)

	inicio := time.Now()
	got := b.Buscar(context.Background(), "peli", Opciones{})
	pasado := time.Since(inicio)

	if pasado > 2*time.Second {
		t.Errorf("tardó %v; debería haberse rendido a los 100ms", pasado)
	}
	if len(got.Resultados) != 1 || got.Resultados[0].Sitio != "Rapido" {
		t.Errorf("Resultados = %+v, quiero solo el del sitio rápido", got.Resultados)
	}
	if len(got.Fallos) != 1 || got.Fallos[0].Sitio != "Lento" {
		t.Errorf("Fallos = %+v, quiero uno del sitio lento", got.Fallos)
	}
}

func TestFiltraPorIdioma(t *testing.T) {
	sitio := &falso{nombre: "Sitio", resultado: []conectores.Resultado{
		res("Sitio", "castellano", titulos.Castellano, titulos.Cal1080p),
		res("Sitio", "latino", titulos.Latino, titulos.Cal1080p),
		res("Sitio", "vose", titulos.VOSE, titulos.Cal1080p),
		res("Sitio", "dual", titulos.Dual, titulos.Cal1080p),
		res("Sitio", "sin marca", titulos.Desconocido, titulos.Cal1080p),
	}}
	b := Nuevo(mudo(), time.Second, sitio)

	got := b.Buscar(context.Background(), "peli", Opciones{})
	if len(got.Resultados) != 1 || got.Resultados[0].Titulo != "castellano" {
		t.Errorf("Resultados = %+v, quiero solo el castellano", got.Resultados)
	}
	// Contar lo descartado es lo que distingue "no hay nada" de "no hay nada
	// en castellano", que para el usuario no es lo mismo.
	if got.Descartados != 4 {
		t.Errorf("Descartados = %d, quiero 4", got.Descartados)
	}

	conDudas := b.Buscar(context.Background(), "peli", Opciones{Dudosos: true})
	if len(conDudas.Resultados) != 3 {
		t.Errorf("con dudosos = %d resultados, quiero 3 (castellano, dual y sin marca)",
			len(conDudas.Resultados))
	}
	// Aun incluyéndolos, el castellano seguro va primero.
	if conDudas.Resultados[0].Titulo != "castellano" {
		t.Errorf("primero = %q, quiero el castellano seguro", conDudas.Resultados[0].Titulo)
	}
}

// Estos sitios buscan por palabras sueltas: pedirles "el padrino" devuelve
// también "El guru de las bodas", porque lleva un "el". Ordenar solo por
// calidad pone ese ruido en 1080p por delante de lo que se buscaba. Es un caso
// real, visto contra EliteTorrent.
func TestLaRelevanciaMandaSobreLaCalidad(t *testing.T) {
	sitio := &falso{nombre: "Sitio", resultado: []conectores.Resultado{
		conObra(res("Sitio", "El guru de las bodas", titulos.Castellano, titulos.Cal1080p)),
		conObra(res("Sitio", "El ultimo padrino", titulos.Castellano, titulos.CalDVDRip)),
		conObra(res("Sitio", "El padrino", titulos.Castellano, titulos.CalDVDRip)),
	}}

	b := Nuevo(mudo(), time.Second, sitio)
	got := b.Buscar(context.Background(), "el padrino", Opciones{})

	quiero := []string{"El padrino", "El ultimo padrino", "El guru de las bodas"}
	for i, q := range quiero {
		if got.Resultados[i].Titulo != q {
			t.Errorf("puesto %d = %q, quiero %q", i, got.Resultados[i].Titulo, q)
		}
	}
}

// conObra deja la obra como la dejaría el analizador de verdad: normalizada.
// Es lo que compara la relevancia exacta.
func conObra(r conectores.Resultado) conectores.Resultado {
	r.Info.Obra = titulos.Obra(r.Titulo)
	return r
}

// Entre dos resultados igual de relevantes sí decide la calidad.
func TestAIgualRelevanciaMandaLaCalidad(t *testing.T) {
	sitio := &falso{nombre: "Sitio", resultado: []conectores.Resultado{
		res("Sitio", "Matrix", titulos.Castellano, titulos.CalDVDRip),
		res("Sitio", "Matrix", titulos.Castellano, titulos.Cal1080p),
	}}
	sitio.resultado[1].Ficha = "https://Sitio/matrix-hd"

	b := Nuevo(mudo(), time.Second, sitio)
	got := b.Buscar(context.Background(), "matrix", Opciones{})

	if got.Resultados[0].Info.Calidad != titulos.Cal1080p {
		t.Errorf("primero = %v, quiero el 1080p", got.Resultados[0].Info.Calidad)
	}
}

// Los capítulos de una serie se leen en orden de emisión. Ordenados por calidad
// sale una lista inservible: 6x9, 6x5, 6x10. Visto en EliteTorrent.
func TestLosCapitulosVanEnOrdenDeEmision(t *testing.T) {
	cap := func(temp, ep int, cal titulos.Calidad) conectores.Resultado {
		r := res("Sitio", fmt.Sprintf("Juego De Tronos %dx%d", temp, ep), titulos.Castellano, cal)
		r.Ficha = fmt.Sprintf("https://sitio/got-%d-%d", temp, ep)
		r.Info.Obra = "juego de tronos"
		r.Info.Temporada, r.Info.Episodio = temp, ep
		return r
	}
	sitio := &falso{nombre: "Sitio", resultado: []conectores.Resultado{
		cap(6, 10, titulos.Cal1080p),
		cap(7, 1, titulos.Cal1080p),
		cap(6, 2, titulos.CalDVDRip),
	}}

	b := Nuevo(mudo(), time.Second, sitio)
	got := b.Buscar(context.Background(), "juego de tronos", Opciones{})

	quiero := []string{"Juego De Tronos 6x2", "Juego De Tronos 6x10", "Juego De Tronos 7x1"}
	for i, q := range quiero {
		if got.Resultados[i].Titulo != q {
			t.Errorf("puesto %d = %q, quiero %q", i, got.Resultados[i].Titulo, q)
		}
	}
}

// Pero el mismo capítulo en dos calidades sigue decidiéndose por calidad.
func TestElMismoCapituloSeOrdenaPorCalidad(t *testing.T) {
	cap := func(cal titulos.Calidad, ficha string) conectores.Resultado {
		r := res("Sitio", "Juego De Tronos 6x2", titulos.Castellano, cal)
		r.Ficha = ficha
		r.Info.Obra = "juego de tronos"
		r.Info.Temporada, r.Info.Episodio = 6, 2
		return r
	}
	sitio := &falso{nombre: "Sitio", resultado: []conectores.Resultado{
		cap(titulos.CalDVDRip, "https://sitio/a"),
		cap(titulos.Cal1080p, "https://sitio/b"),
	}}

	b := Nuevo(mudo(), time.Second, sitio)
	got := b.Buscar(context.Background(), "juego de tronos", Opciones{})

	if got.Resultados[0].Info.Calidad != titulos.Cal1080p {
		t.Errorf("primero = %v, quiero el 1080p", got.Resultados[0].Info.Calidad)
	}
}

func TestFundePorInfoHash(t *testing.T) {
	const magnet = "magnet:?xt=urn:btih:b457beaeb17a343999c335b523b705da6e9277ef&dn=peli"

	unoA := res("Uno", "peli", titulos.Castellano, titulos.Cal1080p)
	unoA.Magnet = magnet
	dosA := res("Dos", "peli con otro nombre", titulos.Castellano, titulos.Cal1080p)
	dosA.Magnet = magnet // El mismo torrent en otro sitio

	uno := &falso{nombre: "Uno", resultado: []conectores.Resultado{unoA}}
	dos := &falso{nombre: "Dos", resultado: []conectores.Resultado{dosA}}

	b := Nuevo(mudo(), time.Second, uno, dos)
	got := b.Buscar(context.Background(), "peli", Opciones{})

	if len(got.Resultados) != 1 {
		t.Errorf("%d resultados, quiero 1: es el mismo torrent en dos sitios", len(got.Resultados))
	}
}

func TestFundeRepetidosDelMismoSitio(t *testing.T) {
	r := res("Uno", "peli", titulos.Castellano, titulos.Cal1080p)
	uno := &falso{nombre: "Uno", resultado: []conectores.Resultado{r, r}}

	b := Nuevo(mudo(), time.Second, uno)
	if got := b.Buscar(context.Background(), "peli", Opciones{}); len(got.Resultados) != 1 {
		t.Errorf("%d resultados, quiero 1", len(got.Resultados))
	}
}

// Dos resultados distintos con la misma pinta no son el mismo: sin infohash,
// fundirlos por título perdería versiones buenas.
func TestNoFundeLoQueSoloSeParece(t *testing.T) {
	uno := &falso{nombre: "Uno", resultado: []conectores.Resultado{
		res("Uno", "peli", titulos.Castellano, titulos.Cal1080p),
	}}
	dos := &falso{nombre: "Dos", resultado: []conectores.Resultado{
		res("Dos", "peli", titulos.Castellano, titulos.Cal1080p),
	}}

	b := Nuevo(mudo(), time.Second, uno, dos)
	if got := b.Buscar(context.Background(), "peli", Opciones{}); len(got.Resultados) != 2 {
		t.Errorf("%d resultados, quiero 2: son de sitios distintos y sin hash", len(got.Resultados))
	}
}

func TestBusquedaSinConectores(t *testing.T) {
	b := Nuevo(mudo(), time.Second)
	got := b.Buscar(context.Background(), "peli", Opciones{})

	if len(got.Resultados) != 0 || len(got.Fallos) != 0 {
		t.Errorf("sin conectores quiero una búsqueda vacía, salió %+v", got)
	}
	if !got.Completa() {
		t.Error("sin conectores no hay nada que falle")
	}
}

func saludDe(t *testing.T, b *Buscador, nombre string) Salud {
	t.Helper()
	for _, s := range b.Salud() {
		if s.Nombre == nombre {
			return s
		}
	}
	t.Fatalf("no hay salud para %q", nombre)
	return Salud{}
}
