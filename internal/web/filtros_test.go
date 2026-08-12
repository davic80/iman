package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/davic80/iman/internal/buscador"
	"github.com/davic80/iman/internal/conectores"
	"github.com/davic80/iman/internal/titulos"
)

// fila arma un resultado ya fundido: el primer sitio es el que manda y los
// demás son los que tienen lo mismo.
func fila(titulo string, calidad titulos.Calidad, sitios ...string) buscador.Resultado {
	uno := func(sitio string) conectores.Resultado {
		info := titulos.Analizar(titulo)
		info.Idioma = titulos.Castellano
		info.Calidad = calidad
		return conectores.Resultado{Sitio: sitio, Titulo: titulo, Info: info}
	}

	r := buscador.Resultado{Resultado: uno(sitios[0])}
	for _, s := range sitios[1:] {
		r.Repetidos = append(r.Repetidos, uno(s))
	}
	return r
}

func lista() []buscador.Resultado {
	return []buscador.Resultado{
		fila("Matrix 1999 1080p", titulos.Cal1080p, "Uno", "Dos"),
		fila("Dune 2021 1080p", titulos.Cal1080p, "Dos"),
		fila("Alien 1979 DVDRip", titulos.CalDVDRip, "Uno"),
		fila("Solaris 1972", titulos.CalidadDesconocida, "Tres"),
	}
}

func titulosDe(rs []buscador.Resultado) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Titulo)
	}
	return out
}

func TestFiltrarPorCalidad(t *testing.T) {
	f := Filtros{Calidad: "1080p"}
	if got := titulosDe(f.aplicar(lista())); len(got) != 2 {
		t.Errorf("aplicar = %v, quiero las dos de 1080p", got)
	}
}

// Lo que no dice de qué calidad es tiene que poder pedirse: si no, esas filas
// no las alcanza ningún filtro.
func TestFiltrarLoQueNoDiceSuCalidad(t *testing.T) {
	f := Filtros{Calidad: sinCalidad}
	got := titulosDe(f.aplicar(lista()))
	if len(got) != 1 || !strings.HasPrefix(got[0], "Solaris") {
		t.Errorf("aplicar = %v, quiero solo Solaris", got)
	}
}

// Una fila fundida es la misma película en varios sitios: si se pide uno de los
// repetidos, la fila tiene que salir igual.
func TestFiltrarPorSitioMiraLosRepetidos(t *testing.T) {
	f := Filtros{Sitio: "Dos"}
	got := titulosDe(f.aplicar(lista()))
	if len(got) != 2 {
		t.Fatalf("aplicar = %v, quiero Matrix (repetida en Dos) y Dune", got)
	}
	if !strings.HasPrefix(got[0], "Matrix") {
		t.Errorf("falta la fundida: %v", got)
	}
}

func TestLosFiltrosSeSuman(t *testing.T) {
	f := Filtros{Calidad: "1080p", Sitio: "Uno"}
	got := titulosDe(f.aplicar(lista()))
	if len(got) != 1 || !strings.HasPrefix(got[0], "Matrix") {
		t.Errorf("aplicar = %v, quiero solo Matrix", got)
	}
}

// Cada botón dice cuántos resultados deja, y para eso hay que contar con los
// otros filtros puestos pero sin el suyo. Si se contara con el suyo, todos los
// números serían iguales a lo que ya se está viendo y no servirían de nada.
func TestLosNumerosCuentanSinElFiltroPropio(t *testing.T) {
	f := Filtros{Calidad: "1080p"}
	b := f.barra(lista(), "/novedades", url.Values{})

	quiero := map[string]int{"1080p": 2, "DVDRip": 1, sinCalidad: 1}
	for _, o := range b.Calidades {
		if n, hay := quiero[o.Valor]; !hay || n != o.Cuantos {
			t.Errorf("calidad %q: %d, quiero %d", o.Valor, o.Cuantos, quiero[o.Valor])
		}
	}
	if len(b.Calidades) != len(quiero) {
		t.Errorf("calidades = %d, quiero %d", len(b.Calidades), len(quiero))
	}

	// Los sitios, en cambio, sí se cuentan con el filtro de calidad aplicado:
	// solo Uno y Dos tienen algo en 1080p.
	sitios := map[string]int{}
	for _, o := range b.Sitios {
		sitios[o.Valor] = o.Cuantos
	}
	if sitios["Uno"] != 1 || sitios["Dos"] != 2 || len(sitios) != 2 {
		t.Errorf("sitios = %v, quiero Uno:1 Dos:2 y nada de Tres", sitios)
	}
}

func TestLasCalidadesSalenDeMejorAPeor(t *testing.T) {
	b := Filtros{}.barra(lista(), "/novedades", url.Values{})

	var orden []string
	for _, o := range b.Calidades {
		orden = append(orden, o.Valor)
	}
	quiero := []string{"1080p", "DVDRip", sinCalidad}
	for i := range quiero {
		if i >= len(orden) || orden[i] != quiero[i] {
			t.Fatalf("orden = %v, quiero %v", orden, quiero)
		}
	}
}

// Un filtro que te borra la búsqueda no lo usa nadie dos veces.
func TestLosEnlacesConservanLoDemas(t *testing.T) {
	q := url.Values{"q": {"matrix"}, "dudosos": {"1"}}
	b := Filtros{}.barra(lista(), "/", q)

	for _, o := range b.Calidades {
		if !strings.Contains(o.URL, "q=matrix") || !strings.Contains(o.URL, "dudosos=1") {
			t.Errorf("el enlace de %q perdió la búsqueda: %s", o.Valor, o.URL)
		}
	}
}

// Volver a pulsar el filtro que ya está puesto lo quita.
func TestElFiltroPuestoSeQuitaPulsandolo(t *testing.T) {
	q := url.Values{"orden": {"sitios"}, "calidad": {"1080p"}}
	b := Filtros{Calidad: "1080p"}.barra(lista(), "/novedades", q)

	var activa *opcion
	for i, o := range b.Calidades {
		if o.Activa {
			activa = &b.Calidades[i]
		}
	}
	if activa == nil {
		t.Fatal("ninguna calidad quedó marcada como activa")
	}
	if strings.Contains(activa.URL, "calidad=") {
		t.Errorf("pulsar la activa debería quitarla, pero lleva a %s", activa.URL)
	}
	if !strings.Contains(activa.URL, "orden=sitios") {
		t.Errorf("quitar la calidad no puede llevarse el orden: %s", activa.URL)
	}
}

// Con un solo valor no hay nada que elegir, así que no se enseña la barra. Pero
// si hay algo filtrado sí, porque si no, no habría manera de quitarlo.
func TestUnSoloValorNoSeOfrece(t *testing.T) {
	solo := []buscador.Resultado{fila("Matrix 1999 1080p", titulos.Cal1080p, "Uno")}

	if b := (Filtros{}).barra(solo, "/novedades", url.Values{}); b.Calidades != nil || b.Sitios != nil {
		t.Errorf("con un solo valor no debería haber botones: %+v", b)
	}

	b := Filtros{Calidad: "1080p"}.barra(solo, "/novedades", url.Values{})
	if b.Calidades == nil {
		t.Error("con un filtro puesto hay que poder quitarlo")
	}
}

func TestLaBusquedaFiltra(t *testing.T) {
	// El ayudante resultado() pone 1080p a todo y la misma ficha a todo, y dos
	// resultados con la misma ficha son el mismo: aquí hacen falta distintos.
	conCalidad := func(titulo string, c titulos.Calidad) conectores.Resultado {
		r := resultado(titulo, titulos.Castellano)
		r.Info.Calidad = c
		r.Ficha = "https://sitio.test/" + titulos.Normalizar(titulo)
		return r
	}
	sitio := &sitioFalso{resultados: []conectores.Resultado{
		conCalidad("Matrix 1999 1080p", titulos.Cal1080p),
		conCalidad("Alien 1979 DVDRip", titulos.CalDVDRip),
	}}
	h := servidorPrueba(t, sitio)

	cuerpo := pedir(t, h, "/?q=cine&calidad=DVDRip").Body.String()
	if strings.Contains(cuerpo, "Matrix") {
		t.Error("el filtro de calidad no quitó Matrix")
	}
	if !strings.Contains(cuerpo, "Alien") {
		t.Error("el filtro de calidad se llevó también lo que pedía")
	}
}

func TestLaPortadaFiltra(t *testing.T) {
	h, _ := servidorConPortada(t,
		novedad("Falso", "El padrino 1972 DVDRip"),
		novedad("Falso", "Matrix 1999 1080p"),
	)

	cuerpo := pedir(t, h, "/novedades?calidad=1080p").Body.String()
	if strings.Contains(cuerpo, "El padrino") {
		t.Error("el filtro no quitó El padrino")
	}
	if !strings.Contains(cuerpo, "Matrix") {
		t.Error("el filtro se llevó lo que pedía")
	}
	if !strings.Contains(cuerpo, "1 película de 2") {
		t.Errorf("el resumen no dice cuántas había, salió %q", primeraLinea(cuerpo))
	}
}

// Quien pulsa "Actualizar" quiere lo mismo que estaba mirando, pero recién
// traído: los filtros tienen que sobrevivir a la vuelta.
func TestActualizarNoSeLlevaLosFiltros(t *testing.T) {
	h, rondin := servidorConPortada(t, novedad("Falso", "Matrix 1999 1080p"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/novedades/refrescar",
		strings.NewReader("orden=sitios&calidad=1080p&sitio=Falso"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)

	destino := rec.Header().Get("Location")
	for _, q := range []string{"orden=sitios", "calidad=1080p", "sitio=Falso"} {
		if !strings.Contains(destino, q) {
			t.Errorf("la vuelta perdió %q: %s", q, destino)
		}
	}
	esperarRonda(t, rondin)
}

// Filtrar hasta quedarse sin nada no es lo mismo que no haber encontrado nada:
// el aviso tiene que decir cómo deshacerlo.
func TestFiltrarHastaQuedarseSinNada(t *testing.T) {
	h, _ := servidorConPortada(t, novedad("Falso", "Matrix 1999 1080p"))

	cuerpo := pedir(t, h, "/novedades?calidad=4K").Body.String()
	if !strings.Contains(cuerpo, "Nada con esos filtros") {
		t.Errorf("falta el aviso, salió %q", primeraLinea(cuerpo))
	}
	if !strings.Contains(cuerpo, "Quitar los filtros") {
		t.Error("el aviso no ofrece deshacerlo")
	}
}
