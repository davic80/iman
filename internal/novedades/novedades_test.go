package novedades

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
)

func callado() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// falso es un sitio de mentira: devuelve lo que se le ponga y cuenta las veces
// que le han preguntado.
type falso struct {
	nombre   string
	rs       []conectores.Resultado
	err      error
	revienta bool
	esperar  chan struct{} // Si no es nil, se queda ahí hasta que se cierre
	llamadas int
}

func (f *falso) Nombre() string { return f.nombre }

func (f *falso) Buscar(context.Context, string) ([]conectores.Resultado, error) {
	return nil, nil
}

func (f *falso) Novedades(context.Context) ([]conectores.Resultado, error) {
	f.llamadas++
	if f.revienta {
		panic("el html ha cambiado")
	}
	if f.esperar != nil {
		<-f.esperar
	}
	return f.rs, f.err
}

// conFicha es un sitio de los que esconden el magnet en la ficha.
type conFicha struct {
	*falso
	falla     bool
	resueltas int
}

func (c *conFicha) Resolver(_ context.Context, r *conectores.Resultado) error {
	c.resueltas++
	if c.falla {
		return errors.New("la ficha no contestó")
	}
	r.Titulo = r.Titulo + " completo"
	r.Magnet = "magnet:?xt=urn:btih:" + r.Ficha[len(r.Ficha)-4:]
	return nil
}

func peli(sitio, titulo string, idioma titulos.Idioma) conectores.Resultado {
	info := titulos.Analizar(titulo)
	info.Idioma = idioma
	return conectores.Resultado{
		Sitio:  sitio,
		Titulo: titulo,
		Ficha:  "https://" + sitio + ".test/pelicula/" + titulos.Normalizar(titulo),
		Info:   info,
	}
}

func castellano(sitio string, titulos_ ...string) []conectores.Resultado {
	var rs []conectores.Resultado
	for _, t := range titulos_ {
		rs = append(rs, peli(sitio, t, titulos.Castellano))
	}
	return rs
}

func TestApuntaLoQueEnseñanLosSitios(t *testing.T) {
	uno := &falso{nombre: "Uno", rs: castellano("Uno", "El padrino", "Matrix")}
	dos := &falso{nombre: "Dos", rs: castellano("Dos", "Dune")}

	r := Nuevo(callado(), nil, uno, dos)
	ronda := r.Rondar(context.Background())

	if ronda.Nuevos != 3 {
		t.Errorf("Nuevos = %d, quiero 3", ronda.Nuevos)
	}
	if len(r.Ultimos()) != 3 {
		t.Errorf("quedaron %d apuntes, quiero 3", len(r.Ultimos()))
	}
	if len(ronda.Fallos) != 0 {
		t.Errorf("fallos inesperados: %v", ronda.Fallos)
	}
}

// La misma película en la rejilla una hora después no vuelve a ser novedad: si
// se le renovara el Visto, las mismas cuatro películas se quedarían clavadas
// arriba de la portada para siempre.
func TestLoYaVistoNoRejuvenece(t *testing.T) {
	uno := &falso{nombre: "Uno", rs: castellano("Uno", "El padrino")}
	r := Nuevo(callado(), nil, uno)

	r.Rondar(context.Background())
	primero := r.Ultimos()[0].Visto

	ronda := r.Rondar(context.Background())
	if ronda.Nuevos != 0 || ronda.Repetidas != 1 {
		t.Errorf("segunda ronda: %d nuevos y %d repetidas, quiero 0 y 1",
			ronda.Nuevos, ronda.Repetidas)
	}
	if visto := r.Ultimos()[0].Visto; !visto.Equal(primero) {
		t.Errorf("Visto pasó de %v a %v", primero, visto)
	}
}

// La portada es de castellano y solo de castellano, sin el interruptor de
// dudosos que sí tiene la búsqueda.
func TestSoloEntraElCastellano(t *testing.T) {
	uno := &falso{nombre: "Uno", rs: []conectores.Resultado{
		peli("Uno", "El padrino", titulos.Castellano),
		peli("Uno", "The Godfather", titulos.Latino),
		peli("Uno", "Dune", titulos.VOSE),
		peli("Uno", "Alien", titulos.Dual),
		peli("Uno", "Solaris", titulos.Desconocido),
	}}

	r := Nuevo(callado(), nil, uno)
	ronda := r.Rondar(context.Background())

	if ronda.Nuevos != 1 || ronda.Descartadas != 4 {
		t.Errorf("%d nuevos y %d descartadas, quiero 1 y 4", ronda.Nuevos, ronda.Descartadas)
	}
	if lista := r.Ultimos(); len(lista) != 1 || lista[0].Titulo != "El padrino" {
		t.Errorf("quedó %+v, quiero solo el castellano", lista)
	}
}

func TestUnSitioCaidoNoEstropeaALosDemas(t *testing.T) {
	roto := &falso{nombre: "Roto", err: errors.New("502")}
	sano := &falso{nombre: "Sano", rs: castellano("Sano", "Dune")}

	r := Nuevo(callado(), nil, roto, sano)
	ronda := r.Rondar(context.Background())

	if len(ronda.Fallos) != 1 || ronda.Fallos[0].Sitio != "Roto" {
		t.Errorf("Fallos = %+v, quiero solo el de Roto", ronda.Fallos)
	}
	if len(r.Ultimos()) != 1 {
		t.Errorf("se perdió lo del sitio sano: %+v", r.Ultimos())
	}
}

// Un parser que revienta con el HTML nuevo de un sitio no puede llevarse por
// delante una tarea de fondo en la que no hay nadie mirando.
func TestUnConectorQueRevientaNoTumbaLaRonda(t *testing.T) {
	malo := &falso{nombre: "Malo", revienta: true}
	bueno := &falso{nombre: "Bueno", rs: castellano("Bueno", "Dune")}

	r := Nuevo(callado(), nil, malo, bueno)
	ronda := r.Rondar(context.Background())

	if len(ronda.Fallos) != 1 || ronda.Fallos[0].Sitio != "Malo" {
		t.Errorf("Fallos = %+v, quiero el panic apuntado como fallo de Malo", ronda.Fallos)
	}
	if len(r.Ultimos()) != 1 {
		t.Errorf("se perdió lo del otro sitio: %+v", r.Ultimos())
	}
}

func TestSeAbrenLasFichasDeLosApuntesNuevos(t *testing.T) {
	sitio := &conFicha{falso: &falso{
		nombre: "Uno",
		rs:     castellano("Uno", "El padrino", "Matrix", "Dune"),
	}}

	r := Nuevo(callado(), nil, sitio)
	ronda := r.Rondar(context.Background())

	if ronda.Completadas != 3 {
		t.Errorf("Completadas = %d, quiero 3", ronda.Completadas)
	}
	for _, a := range r.Ultimos() {
		if !a.Resuelto {
			t.Errorf("%q quedó a medias", a.Titulo)
		}
		if a.Magnet == "" {
			t.Errorf("%q no trae magnet, que es para lo que se abre la ficha", a.Titulo)
		}
	}

	// Y en la ronda siguiente no se vuelven a abrir: ya están completas.
	r.Rondar(context.Background())
	if sitio.resueltas != 3 {
		t.Errorf("se abrieron %d fichas en dos rondas, quiero 3", sitio.resueltas)
	}
}

// Abrir fichas cuesta una petición por película, así que se hacen unas pocas
// por ronda y las demás esperan.
func TestSoloSeAbrenUnasCuantasFichasPorRonda(t *testing.T) {
	sitio := &conFicha{falso: &falso{
		nombre: "Uno",
		rs:     castellano("Uno", "El padrino", "Matrix", "Dune", "Alien", "Solaris"),
	}}

	r := Nuevo(callado(), nil, sitio)
	r.Fichas = 2

	if ronda := r.Rondar(context.Background()); ronda.Completadas != 2 {
		t.Errorf("Completadas = %d, quiero 2", ronda.Completadas)
	}
	if ronda := r.Rondar(context.Background()); ronda.Completadas != 2 {
		t.Errorf("segunda ronda: Completadas = %d, quiero otras 2", ronda.Completadas)
	}

	var resueltos int
	for _, a := range r.Ultimos() {
		if a.Resuelto {
			resueltos++
		}
	}
	if resueltos != 4 {
		t.Errorf("van %d resueltos tras dos rondas, quiero 4", resueltos)
	}
}

// Una ficha rota no se puede reintentar cada hora mientras siga en la lista.
func TestUnaFichaQueFallaSeDejaEnPaz(t *testing.T) {
	sitio := &conFicha{
		falso: &falso{nombre: "Uno", rs: castellano("Uno", "El padrino")},
		falla: true,
	}

	r := Nuevo(callado(), nil, sitio)
	for i := 0; i < 5; i++ {
		r.Rondar(context.Background())
	}

	if sitio.resueltas != intentosPorFicha {
		t.Errorf("se intentó %d veces, quiero %d", sitio.resueltas, intentosPorFicha)
	}
	// Y aunque no se pudiera completar, la película sigue en la portada: el
	// título de la rejilla es peor que el de la ficha, pero es algo.
	if len(r.Ultimos()) != 1 {
		t.Errorf("se perdió el apunte por no poder abrir su ficha")
	}
}

// La rejilla trae menos datos que la ficha, así que refrescar un apunte ya
// resuelto con lo que dice la rejilla sería cambiar bueno por malo.
func TestLaRejillaNoPisaLoQueSacoLaFicha(t *testing.T) {
	sitio := &conFicha{falso: &falso{
		nombre: "Uno",
		rs:     castellano("Uno", "El padrino"),
	}}

	r := Nuevo(callado(), nil, sitio)
	r.Rondar(context.Background())
	r.Rondar(context.Background())

	a := r.Ultimos()[0]
	if a.Titulo != "El padrino completo" {
		t.Errorf("Titulo = %q, quiero el que sacó la ficha", a.Titulo)
	}
	if a.Magnet == "" {
		t.Error("se perdió el magnet en la segunda ronda")
	}
}

func TestSeTiraLoViejo(t *testing.T) {
	r := Nuevo(callado(), nil)
	r.Retencion = 24 * time.Hour

	ahora := time.Now().UTC()
	r.apuntes = map[string]Apunte{
		"nuevo": {Resultado: peli("Uno", "Dune", titulos.Castellano), Visto: ahora},
		"viejo": {
			Resultado: peli("Uno", "Ben-Hur", titulos.Castellano),
			Visto:     ahora.Add(-48 * time.Hour),
		},
	}
	r.podar()

	if len(r.apuntes) != 1 {
		t.Fatalf("quedaron %d apuntes, quiero 1", len(r.apuntes))
	}
	if _, sigue := r.apuntes["nuevo"]; !sigue {
		t.Error("se tiró el nuevo en vez del viejo")
	}
}

// La retención sola no basta: un sitio que publique mil cosas en un día dejaría
// la portada ilegible y el fichero enorme.
func TestSeRespetaElTope(t *testing.T) {
	uno := &falso{nombre: "Uno", rs: castellano("Uno",
		"El padrino", "Matrix", "Dune", "Alien", "Solaris")}

	r := Nuevo(callado(), nil, uno)
	r.Maximo = 3
	r.Rondar(context.Background())

	if len(r.Ultimos()) != 3 {
		t.Errorf("quedaron %d apuntes, quiero 3", len(r.Ultimos()))
	}
}

func TestLoMasNuevoVaPrimero(t *testing.T) {
	ahora := time.Now().UTC()
	as := []Apunte{
		{Resultado: peli("Uno", "Viejo", titulos.Castellano), Visto: ahora.Add(-2 * time.Hour)},
		{Resultado: peli("Uno", "Nuevo", titulos.Castellano), Visto: ahora},
		{Resultado: peli("Uno", "Medio", titulos.Castellano), Visto: ahora.Add(-time.Hour)},
	}
	ordenarPorFecha(as)

	quiero := []string{"Nuevo", "Medio", "Viejo"}
	for i, q := range quiero {
		if as[i].Titulo != q {
			t.Errorf("en la posición %d hay %q, quiero %q", i, as[i].Titulo, q)
		}
	}
}

// El botón de la portada y el reloj llaman al mismo sitio: dos rondas a la vez
// serían el doble de peticiones a los mismos sitios para el mismo resultado.
func TestNoSeSolapanDosRondas(t *testing.T) {
	puerta := make(chan struct{})
	lento := &falso{
		nombre:  "Lento",
		rs:      castellano("Lento", "Dune"),
		esperar: puerta,
	}

	r := Nuevo(callado(), nil, lento)

	acabada := make(chan struct{})
	go func() {
		r.Rondar(context.Background())
		close(acabada)
	}()

	// Se espera a que la primera ronda esté dentro del sitio lento.
	esperarA(t, func() bool { return r.Rondando() })

	r.Rondar(context.Background()) // No debería quedarse esperando

	close(puerta)
	<-acabada

	if lento.llamadas != 1 {
		t.Errorf("se preguntó al sitio %d veces, quiero 1", lento.llamadas)
	}
}

func esperarA(t *testing.T, cumple func() bool) {
	t.Helper()
	limite := time.Now().Add(2 * time.Second)
	for !cumple() {
		if time.Now().After(limite) {
			t.Fatal("se agotó la espera")
		}
		time.Sleep(time.Millisecond)
	}
}

// Sin esto, cada despliegue dejaría la portada vacía hasta la primera ronda.
func TestGuardaYRecupera(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "sub", "novedades.json")
	almacen := NuevoAlmacen(ruta)

	uno := &falso{nombre: "Uno", rs: castellano("Uno", "El padrino", "Matrix")}
	r := Nuevo(callado(), almacen, uno)
	r.Rondar(context.Background())

	otro := Nuevo(callado(), almacen, uno)
	otro.Restaurar()

	antes, despues := r.Ultimos(), otro.Ultimos()
	if len(despues) != len(antes) {
		t.Fatalf("se recuperaron %d apuntes de %d", len(despues), len(antes))
	}
	for i := range antes {
		if antes[i].Titulo != despues[i].Titulo || !antes[i].Visto.Equal(despues[i].Visto) {
			t.Errorf("el apunte %d cambió al recuperarlo: %+v vs %+v",
				i, antes[i], despues[i])
		}
		if antes[i].Info.Idioma != despues[i].Info.Idioma {
			t.Errorf("%q perdió el idioma al recuperarlo", antes[i].Titulo)
		}
	}
}

// Un fichero de otra versión no se puede interpretar a medias: los idiomas se
// guardan como números, y leerlos con otra tabla diría cosas falsas.
func TestUnFormatoViejoSeTira(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "novedades.json")
	if err := escribir(ruta, `{"formato":0,"apuntes":[{"Titulo":"Dune"}]}`); err != nil {
		t.Fatal(err)
	}

	if _, err := NuevoAlmacen(ruta).Cargar(); err == nil {
		t.Error("se aceptó un fichero de otro formato")
	}

	// Y el rondín arranca igual, con la lista vacía.
	r := Nuevo(callado(), NuevoAlmacen(ruta))
	r.Restaurar()
	if len(r.Ultimos()) != 0 {
		t.Errorf("se coló algo del fichero viejo: %+v", r.Ultimos())
	}
}

// Un fichero a medio escribir tampoco puede impedir que Imán arranque: esto es
// un caché, y perderlo solo cuesta una ronda.
func TestUnFicheroRotoNoImpideArrancar(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "novedades.json")
	if err := escribir(ruta, `{"formato":1,"apuntes":[{"Tit`); err != nil {
		t.Fatal(err)
	}

	r := Nuevo(callado(), NuevoAlmacen(ruta))
	r.Restaurar()
	if len(r.Ultimos()) != 0 {
		t.Errorf("salió algo de un fichero roto: %+v", r.Ultimos())
	}
}

func TestSinAlmacenNoSeGuardaNada(t *testing.T) {
	uno := &falso{nombre: "Uno", rs: castellano("Uno", "Dune")}
	r := Nuevo(callado(), nil, uno)

	if ronda := r.Rondar(context.Background()); ronda.Nuevos != 1 {
		t.Errorf("la ronda falló sin almacén: %+v", ronda)
	}
}

func escribir(ruta, contenido string) error {
	return os.WriteFile(ruta, []byte(contenido), 0o644)
}

// Los sitios publican de más nuevo a más viejo. Dentro de una ronda, donde todo
// comparte Visto, ese orden es la única señal de qué es más reciente; sin él la
// portada del primer arranque sale en orden alfabético.
func TestSeConservaElOrdenDeLaRejilla(t *testing.T) {
	uno := &falso{nombre: "Uno", rs: castellano("Uno", "Zulú", "Amanecer", "Matrix")}

	r := Nuevo(callado(), nil, uno)
	r.Rondar(context.Background())

	quiero := []string{"Zulú", "Amanecer", "Matrix"}
	for i, a := range r.Ultimos() {
		if a.Titulo != quiero[i] {
			t.Errorf("en la posición %d hay %q, quiero %q", i, a.Titulo, quiero[i])
		}
	}
}

// Un sitio que falla deja su hueco en la portada hasta la ronda siguiente, así
// que se vuelve antes. En el primer arranque es lo normal: se llega antes de que
// el vigilante de dominios sepa dónde vive cada sitio.
func TestSiFallaAlgunSitioSeReintentaAntes(t *testing.T) {
	roto := &falso{nombre: "Roto", err: errors.New("502")}
	sano := &falso{nombre: "Sano", rs: castellano("Sano", "Dune")}

	r := Nuevo(callado(), nil, roto, sano)
	r.Cada = time.Hour
	r.Reintento = time.Minute

	var malas int
	if espera := r.siguiente(r.Rondar(context.Background()), &malas); espera != time.Minute {
		t.Errorf("espera = %v con un sitio caído, quiero el reintento corto", espera)
	}

	r = Nuevo(callado(), nil, sano)
	r.Cada = time.Hour
	r.Reintento = time.Minute

	malas = 0
	if espera := r.siguiente(r.Rondar(context.Background()), &malas); espera != time.Hour {
		t.Errorf("espera = %v sin fallos, quiero la ronda normal", espera)
	}
}

// Pero un sitio que sigue caído no se arregla porque le preguntemos más: la
// espera se va doblando hasta la ronda normal, y ahí se queda.
func TestElReintentoSeVaEspaciando(t *testing.T) {
	roto := &falso{nombre: "Roto", err: errors.New("502")}

	r := Nuevo(callado(), nil, roto)
	r.Cada = time.Hour
	r.Reintento = 5 * time.Minute

	quiero := []time.Duration{
		5 * time.Minute,
		10 * time.Minute,
		20 * time.Minute,
		40 * time.Minute,
		time.Hour,
		time.Hour,
	}

	var malas int
	for i, q := range quiero {
		espera := r.siguiente(r.Rondar(context.Background()), &malas)
		if espera != q {
			t.Fatalf("ronda %d: espera = %v, quiero %v", i+1, espera, q)
		}
	}

	// Y en cuanto el sitio vuelve se recupera el ritmo de siempre, sin arrastrar
	// nada de lo anterior: si mañana falla otra vez, se le vuelve a dar la
	// oportunidad rápida.
	roto.err = nil
	roto.rs = castellano("Roto", "Dune")
	if espera := r.siguiente(r.Rondar(context.Background()), &malas); espera != time.Hour {
		t.Errorf("espera = %v con el sitio ya sano, quiero la ronda normal", espera)
	}
	roto.err = errors.New("502")
	if espera := r.siguiente(r.Rondar(context.Background()), &malas); espera != 5*time.Minute {
		t.Errorf("espera = %v al primer fallo nuevo, quiero el reintento corto", espera)
	}
}
