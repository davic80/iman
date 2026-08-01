package conectores

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClienteDocumento(t *testing.T) {
	var ua, idioma string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua, idioma = r.Header.Get("User-Agent"), r.Header.Get("Accept-Language")
		io.WriteString(w, `<html><body><h1>hola</h1></body></html>`)
	}))
	defer srv.Close()

	doc, err := NuevoCliente(0).Documento(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Documento: %v", err)
	}
	if got := doc.Find("h1").Text(); got != "hola" {
		t.Errorf("h1 = %q", got)
	}
	// Sin User-Agent de navegador varios de estos sitios contestan 403.
	if !strings.HasPrefix(ua, "Mozilla/") {
		t.Errorf("User-Agent = %q, quiero uno de navegador", ua)
	}
	if !strings.Contains(idioma, "es") {
		t.Errorf("Accept-Language = %q, quiero español", idioma)
	}
}

func TestClienteErrorHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := NuevoCliente(0).Documento(context.Background(), srv.URL); err == nil {
		t.Fatal("quiero error con un 403, no salió ninguno")
	}
}

// Pedir varias páginas seguidas a estos sitios les hace dar timeouts. El
// cliente tiene que espaciarlas él solo.
func TestClienteEspaciaLasPeticiones(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<html></html>")
	}))
	defer srv.Close()

	const intervalo = 60 * time.Millisecond
	c := NuevoCliente(intervalo)

	inicio := time.Now()
	for range 3 {
		if _, err := c.Documento(context.Background(), srv.URL); err != nil {
			t.Fatalf("Documento: %v", err)
		}
	}
	// Tres peticiones son dos esperas: la primera sale enseguida.
	if pasado := time.Since(inicio); pasado < 2*intervalo {
		t.Errorf("3 peticiones tardaron %v, quiero al menos %v", pasado, 2*intervalo)
	}
}

// Dominios distintos no tienen por qué esperarse entre ellos: el buscador
// lanza todos los sitios a la vez y sería tirar el tiempo del usuario.
func TestClienteNoEspaciaEntreDominiosDistintos(t *testing.T) {
	uno := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer uno.Close()
	dos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer dos.Close()

	c := NuevoCliente(2 * time.Second)
	inicio := time.Now()
	for _, srv := range []*httptest.Server{uno, dos} {
		if _, err := c.Documento(context.Background(), srv.URL); err != nil {
			t.Fatalf("Documento: %v", err)
		}
	}
	if pasado := time.Since(inicio); pasado > time.Second {
		t.Errorf("dos dominios distintos tardaron %v; no deberían esperarse", pasado)
	}
}

// La espera del turno tiene que poder cancelarse: si el buscador se queda sin
// presupuesto de tiempo, no vale seguir dormido.
func TestClienteRespetaElContexto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	c := NuevoCliente(10 * time.Second)
	if _, err := c.Documento(context.Background(), srv.URL); err != nil {
		t.Fatalf("primera petición: %v", err)
	}

	ctx, cancelar := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelar()

	inicio := time.Now()
	if _, err := c.Documento(ctx, srv.URL); err == nil {
		t.Fatal("quiero error al cancelarse el contexto")
	}
	if pasado := time.Since(inicio); pasado > time.Second {
		t.Errorf("tardó %v en rendirse; debería cortar al vencer el contexto", pasado)
	}
}

// Un sitio que se ponga a escupir megas no debe llevarse por delante la
// memoria del proceso.
func TestClienteCortaRespuestasEnormes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trozo := strings.Repeat("a", 1<<20)
		for range 8 {
			if _, err := io.WriteString(w, trozo); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	cuerpo, err := NuevoCliente(0).Traer(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Traer: %v", err)
	}
	defer cuerpo.Close()

	n, err := io.Copy(io.Discard, cuerpo)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if n > MaxCuerpo {
		t.Errorf("se leyeron %d bytes, el tope es %d", n, MaxCuerpo)
	}
}
