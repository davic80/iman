package buscador

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/davic80/iman/internal/conectores"
)

// Resolver pide a un sitio los enlaces de un resultado concreto.
//
// Va aparte de Buscar porque los sitios esconden el magnet en la ficha y
// pedirlas todas durante la búsqueda sería multiplicar las peticiones por el
// número de resultados para que el usuario use una sola.
func (b *Buscador) Resolver(ctx context.Context, sitio, ficha string) (conectores.Resultado, error) {
	c := b.conector(sitio)
	if c == nil {
		return conectores.Resultado{}, fmt.Errorf("no hay ningún sitio llamado %q", sitio)
	}
	resolutor, puede := c.(conectores.Resolutor)
	if !puede {
		return conectores.Resultado{}, fmt.Errorf("%s no sabe abrir fichas", sitio)
	}

	// La ficha no se valida aquí: quien sabe qué URLs son suyas es el propio
	// conector, que es el que conoce su dominio de hoy.
	r := conectores.Resultado{Sitio: sitio, Ficha: ficha}
	if err := resolutor.Resolver(ctx, &r); err != nil {
		return r, err
	}
	return r, nil
}

// Torrent trae el fichero .torrent de una ficha, junto con el nombre con el que
// debería guardarse.
//
// Lo pide el servidor, no el navegador: así quien busca no acaba conectándose
// al sitio de torrents ni apareciendo en sus registros. Hay que cerrar el
// cuerpo devuelto.
func (b *Buscador) Torrent(ctx context.Context, sitio, ficha string) (io.ReadCloser, string, error) {
	r, err := b.Resolver(ctx, sitio, ficha)
	if err != nil {
		return nil, "", err
	}
	if r.Torrent == "" {
		return nil, "", fmt.Errorf("%s no ofrece .torrent para esa ficha", sitio)
	}

	c := b.conector(sitio)
	descargador, puede := c.(conectores.Descargador)
	if !puede {
		return nil, "", fmt.Errorf("%s no sabe descargar ficheros", sitio)
	}

	cuerpo, err := descargador.Descargar(ctx, r.Torrent)
	if err != nil {
		return nil, "", err
	}
	return cuerpo, nombreFichero(r.Torrent), nil
}

func (b *Buscador) conector(nombre string) conectores.Conector {
	for _, c := range b.conectores {
		if c.Nombre() == nombre {
			return c
		}
	}
	return nil
}

// nombreFichero saca un nombre de fichero presentable de la URL del .torrent.
// Se limpia porque acaba en una cabecera Content-Disposition y viene de una
// página ajena.
func nombreFichero(dir string) string {
	nombre := path.Base(dir)
	if i := strings.IndexAny(nombre, "?#"); i >= 0 {
		nombre = nombre[:i]
	}
	nombre = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`"\/:*?<>|`, r) {
			return -1
		}
		return r
	}, nombre)
	if nombre == "" || nombre == "." {
		return "descarga.torrent"
	}
	if !strings.HasSuffix(strings.ToLower(nombre), ".torrent") {
		nombre += ".torrent"
	}
	return nombre
}
