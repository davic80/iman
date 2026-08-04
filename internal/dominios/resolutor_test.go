package dominios

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/davic80/iman/internal/conectores"
)

func mudo() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// paginaBuena es lo que devuelve el sitio auténtico: lleva sus marcas y sabe
// buscar.
const paginaBuena = `<html><title>Resultados de matrix - EliteTorrent</title>
<ul class="miniboxs-ficha"><li>...</li></ul></html>`

// paginaParking es un dominio caducado que alguien ha comprado.
const paginaParking = `<html><title>elitetorrent.wf</title>
<div class="miniboxs-ficha">Buy this domain</div>
<script src="/sk-park.php"></script></html>`

// paginaClon es la peligrosa: ha copiado el diseño entero, marcas incluidas,
// pero no tiene torrents que devolver.
const paginaClon = paginaBuena

// sitioFalso es un conector mudable de mentira. Sirve lo que se le diga para
// cada dominio y apunta a quién le han preguntado.
type sitioFalso struct {
	nombre   string
	semillas []string
	canal    string
	huella   conectores.Huella

	mu      sync.Mutex
	base    string
	paginas map[string]conectores.Sondeo
	sondeos []string
}

func (s *sitioFalso) Nombre() string { return s.nombre }

func (s *sitioFalso) Buscar(context.Context, string) ([]conectores.Resultado, error) {
	return nil, nil
}

func (s *sitioFalso) Dominio() string { return anfitrion(s.Base()) }

func (s *sitioFalso) Base() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.base
}

func (s *sitioFalso) Mudar(base string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.base = base
}

func (s *sitioFalso) Semillas() []string { return s.semillas }

func (s *sitioFalso) Huella() conectores.Huella {
	if s.huella.Consulta == "" {
		return conectores.Huella{
			Contiene:    []string{"miniboxs-ficha"},
			Consulta:    "matrix",
			MinAciertos: 3,
		}
	}
	return s.huella
}

func (s *sitioFalso) Sondear(_ context.Context, base string) (conectores.Sondeo, error) {
	s.mu.Lock()
	s.sondeos = append(s.sondeos, base)
	sondeo, ok := s.paginas[base]
	s.mu.Unlock()

	if !ok {
		return conectores.Sondeo{}, errors.New("no responde")
	}
	return sondeo, nil
}

func (s *sitioFalso) Canal() string { return s.canal }

func (s *sitioFalso) preguntados() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sondeos...)
}

// clienteFalso responde redirecciones y páginas de memoria.
type clienteFalso struct {
	destinos map[string]string
	cuerpos  map[string]string
}

func (c *clienteFalso) Destino(_ context.Context, dir string) (string, error) {
	if d, ok := c.destinos[dir]; ok {
		return d, nil
	}
	return "", errors.New("no responde")
}

func (c *clienteFalso) Traer(_ context.Context, dir string) (io.ReadCloser, error) {
	if b, ok := c.cuerpos[dir]; ok {
		return io.NopCloser(strings.NewReader(b)), nil
	}
	return nil, errors.New("no responde")
}

func buena(n int) conectores.Sondeo {
	return conectores.Sondeo{HTML: paginaBuena, Resultados: n}
}

func TestNoSeMudaSiElDominioEnUsoVaBien(t *testing.T) {
	sitio := &sitioFalso{
		nombre:   "Sitio",
		base:     "https://sitio.uno",
		semillas: []string{"https://sitio.uno", "https://sitio.dos"},
		paginas: map[string]conectores.Sondeo{
			"https://sitio.uno": buena(14),
			"https://sitio.dos": buena(14),
		},
	}
	r := Nuevo(&clienteFalso{}, mudo(), nil, sitio)

	if err := r.Revisar1(context.Background(), sitio); err != nil {
		t.Fatalf("Revisar1: %v", err)
	}
	if got := sitio.Base(); got != "https://sitio.uno" {
		t.Errorf("base = %q, no debería haberse movido", got)
	}
	// Y sobre todo: una sola petición. Revalidar cada seis horas no puede
	// costar una ronda entera contra todos los dominios conocidos.
	if p := sitio.preguntados(); len(p) != 1 {
		t.Errorf("se hicieron %d sondeos (%v), quiero 1", len(p), p)
	}
}

func TestAdoptaElDominioNuevoCuandoElViejoMuere(t *testing.T) {
	sitio := &sitioFalso{
		nombre:   "Sitio",
		base:     "https://sitio.muerto",
		semillas: []string{"https://sitio.muerto", "https://sitio.vivo"},
		paginas: map[string]conectores.Sondeo{
			"https://sitio.vivo": buena(14),
		},
	}
	r := Nuevo(&clienteFalso{}, mudo(), nil, sitio)

	if err := r.Revisar1(context.Background(), sitio); err != nil {
		t.Fatalf("Revisar1: %v", err)
	}
	if got := sitio.Base(); got != "https://sitio.vivo" {
		t.Errorf("base = %q, quiero el dominio vivo", got)
	}
	if c := r.Comprobaciones()["Sitio"]; c.Origen != "semilla" {
		t.Errorf("Origen = %q, quiero saber por dónde apareció", c.Origen)
	}
}

// Un parking responde 200 y hasta puede copiar un trozo del diseño. Si esto
// falla, Imán acaba mandando al usuario a la página de quien compró el dominio.
func TestRechazaUnParking(t *testing.T) {
	sitio := &sitioFalso{
		nombre:   "Sitio",
		base:     "https://sitio.muerto",
		semillas: []string{"https://sitio.okupado"},
		paginas: map[string]conectores.Sondeo{
			// Marcas del sitio sí, pero también las del parking.
			"https://sitio.okupado": {HTML: paginaParking, Resultados: 5},
		},
	}
	r := Nuevo(&clienteFalso{}, mudo(), nil, sitio)

	if err := r.Revisar1(context.Background(), sitio); err == nil {
		t.Fatal("se aceptó un parking")
	}
	if got := sitio.Base(); got != "https://sitio.muerto" {
		t.Errorf("base = %q, no debería haberse mudado al parking", got)
	}
	// Y que caiga por lo que tiene que caer: si lo rechazara por otra cosa,
	// este test estaría dando por buena una defensa que no existe.
	if _, err := r.verificar(context.Background(), sitio, "https://sitio.okupado"); !errors.Is(err, ErrParking) {
		t.Errorf("error = %v, quiero ErrParking", err)
	}
}

// El caso que de verdad importa: un clon perfecto del diseño. Pasa las marcas
// positivas y las negativas, y solo cae por lo único que no se puede falsear:
// no tiene torrents que devolver.
func TestRechazaUnClonQueNoSabeBuscar(t *testing.T) {
	sitio := &sitioFalso{
		nombre:   "Sitio",
		base:     "https://sitio.muerto",
		semillas: []string{"https://sitio.clonado"},
		paginas: map[string]conectores.Sondeo{
			"https://sitio.clonado": {HTML: paginaClon, Resultados: 0},
		},
	}
	r := Nuevo(&clienteFalso{}, mudo(), nil, sitio)

	if err := r.Revisar1(context.Background(), sitio); err == nil {
		t.Fatal("se aceptó un clon que no devuelve resultados")
	}
	if got := sitio.Base(); got != "https://sitio.muerto" {
		t.Errorf("base = %q, no debería haberse mudado al clon", got)
	}
	// Aquí no valen ni las marcas positivas ni las negativas: el clon las pasa
	// todas. Lo único que lo delata es la prueba funcional.
	if _, err := r.verificar(context.Background(), sitio, "https://sitio.clonado"); !errors.Is(err, ErrSinAciertos) {
		t.Errorf("error = %v, quiero que lo delate la búsqueda de prueba", err)
	}
}

// Y el reverso: pocos resultados no es lo mismo que ninguno, pero tampoco
// basta. El mínimo lo pone la huella de cada sitio.
func TestRechazaSiNoLlegaAlMinimoDeAciertos(t *testing.T) {
	sitio := &sitioFalso{nombre: "Sitio", base: "https://sitio.flojo",
		paginas: map[string]conectores.Sondeo{"https://sitio.flojo": buena(2)}}
	r := Nuevo(&clienteFalso{}, mudo(), nil, sitio)

	_, err := r.verificar(context.Background(), sitio, "https://sitio.flojo")
	if !errors.Is(err, ErrSinAciertos) {
		t.Errorf("error = %v, quiero ErrSinAciertos", err)
	}
}

func TestSigueLaRedireccionDelDominioViejo(t *testing.T) {
	sitio := &sitioFalso{
		nombre:   "Sitio",
		base:     "https://sitio.viejo",
		semillas: []string{"https://sitio.viejo"},
		paginas: map[string]conectores.Sondeo{
			"https://sitio.nuevo": buena(14),
		},
	}
	cliente := &clienteFalso{destinos: map[string]string{
		"https://sitio.viejo": "https://sitio.nuevo",
	}}
	r := Nuevo(cliente, mudo(), nil, sitio)

	if err := r.Revisar1(context.Background(), sitio); err != nil {
		t.Fatalf("Revisar1: %v", err)
	}
	if got := sitio.Base(); got != "https://sitio.nuevo" {
		t.Errorf("base = %q, quiero el destino de la redirección", got)
	}
	if c := r.Comprobaciones()["Sitio"]; !strings.HasPrefix(c.Origen, "redirección") {
		t.Errorf("Origen = %q, quiero que diga que fue por redirección", c.Origen)
	}
}

func TestEncuentraElDominioPorTelegram(t *testing.T) {
	sitio := &sitioFalso{
		nombre: "Sitio",
		base:   "https://sitio.viejo",
		canal:  "canaldelsitio",
		paginas: map[string]conectores.Sondeo{
			"https://sitio.recien.anunciado": buena(14),
		},
	}
	cliente := &clienteFalso{cuerpos: map[string]string{
		"https://t.me/s/canaldelsitio": `
			<div class="tgme_widget_message_text">Nuevo dominio: sitio.antiguo</div>
			<div class="tgme_widget_message_text">Ahora estamos en sitio.recien.anunciado</div>`,
	}}
	r := Nuevo(cliente, mudo(), nil, sitio)

	if err := r.Revisar1(context.Background(), sitio); err != nil {
		t.Fatalf("Revisar1: %v", err)
	}
	if got := sitio.Base(); got != "https://sitio.recien.anunciado" {
		t.Errorf("base = %q, quiero el último dominio anunciado en el canal", got)
	}
}

func TestElContadorProponeLosSiguientes(t *testing.T) {
	got := siguientesDelContador("https://www43.mejortorrent.eu", 3)
	quiero := []string{
		"https://www44.mejortorrent.eu",
		"https://www45.mejortorrent.eu",
		"https://www46.mejortorrent.eu",
	}
	if fmt.Sprint(got) != fmt.Sprint(quiero) {
		t.Errorf("got = %v, quiero %v", got, quiero)
	}
	// Un dominio sin contador no genera candidatos inventados.
	if got := siguientesDelContador("https://www.elitetorrent.wf", 3); got != nil {
		t.Errorf("got = %v, un dominio sin número no debería proponer nada", got)
	}
}

func TestLosCandidatosNoSeRepiten(t *testing.T) {
	sitio := &sitioFalso{
		nombre:   "Sitio",
		base:     "https://sitio.uno",
		semillas: []string{"https://sitio.uno/", "https://sitio.uno", "sitio.uno"},
	}
	cliente := &clienteFalso{destinos: map[string]string{
		"https://sitio.uno": "https://sitio.uno",
	}}
	r := Nuevo(cliente, mudo(), nil, sitio)

	cs := r.candidatos(context.Background(), sitio, "https://sitio.uno")
	if len(cs) != 1 {
		t.Errorf("candidatos = %v, quiero uno solo: son el mismo escrito de tres formas", cs)
	}
}

// Un candidato sacado de un HTML ajeno no puede acabar en una petición a
// cualquier cosa.
func TestNoSeAceptanEsquemasRaros(t *testing.T) {
	for _, malo := range []string{"file:///etc/passwd", "javascript:alert(1)", "", "   "} {
		if got := normalizarBase(malo); got != "" {
			t.Errorf("normalizarBase(%q) = %q, quiero que se descarte", malo, got)
		}
	}
}
