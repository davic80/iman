# Imán 🧲

Buscador de torrents **en castellano de España**. Consulta en vivo varios sitios
españoles, filtra el idioma de verdad y te da el magnet o el `.torrent`.

Sin latino. Sin VOSE. Sin resultados que dicen "Spanish" y no lo son.

> **Estado: fase 4.** Funciona de punta a punta con tres sitios
> (**EliteTorrent**, **DonTorrent** y **DivxTotal**): buscas, se consultan en
> paralelo, filtra el idioma, ordena y te da el magnet o el `.torrent`. El
> resolutor de dominios ya está montado, así que cuando un sitio se mude Imán lo
> encontrará solo. Y en `/novedades` está lo último que han subido, sin buscar
> nada. Ver [PLAN.md](PLAN.md).

## Por qué

Jackett y Prowlarr tienen unas 550 definiciones de indexers entre los dos. En
español hay 14, de las cuales **12 son trackers privados** y ninguna es pública
y generalista. Los grandes sitios españoles no están en ninguno de los dos.

Y no es por dejadez: el formato YAML de esas herramientas describe una lista
**estática** de dominios, mantenida a mano vía pull request. Los sitios
españoles cambian de dominio cada pocas semanas, montan interstitials con
JavaScript y a veces sus dominios abandonados los compra un parking que se hace
pasar por ellos. Nada de eso cabe en un YAML.

Tampoco sirve buscar "castellano" en los sitios internacionales. Lo medimos:
sobre **490 resultados** de The Pirate Bay para cinco películas grandes,
**ninguno** venía marcado como castellano explícito.

De ahí Imán.

## Cómo funciona

| Pieza | Qué hace |
|---|---|
| Resolutor de dominios | Descubre solo qué dominio está vivo hoy y verifica que es el sitio auténtico, no un parking |
| Conectores | Uno por sitio. Buscan y parsean |
| Motor de fan-out | Los lanza en paralelo con presupuesto de tiempo y aísla los que fallan |
| Clasificador | Saca idioma, calidad, año y episodio del título. Aquí vive el filtro de castellano |
| Deduplicador | Junta en una fila el mismo torrent de varios sitios, sin perder el enlace de ninguno |
| Rondín de novedades | Cada hora apunta lo que han subido los sitios, para poder verlo sin buscar |
| Filtros | Recortan por calidad y por sitio, en la URL para que el enlace se pueda compartir |

Sin base de datos. Un solo contenedor. Plantillas de Go y htmx, sin build de
JavaScript.

### Sitios conectados

| Sitio | Dominio de partida | Particularidades |
|---|---|---|
| EliteTorrent | `www.elitetorrent.wf` | WordPress plano. Publica semillas y magnet |
| DonTorrent | `tomadivx.net` | Exige un `Referer` suyo para buscar. Ni magnets ni semillas: solo `.torrent` |
| DivxTotal | `divxtotal.foo` | El botón de descarga va a un acortador; el enlace bueno está en un `data-src` en base64. Declara el idioma |

El dominio de partida es solo por dónde se empieza: el resolutor lo cambia si
deja de responder. En DonTorrent no es el dominio que el sitio anuncia como
oficial, porque ese veta el rango de IPs del servidor; TomaDivx es el mismo
sitio, el mismo catálogo y la misma plantilla.

Cuando dos sitios publican lo mismo, va en una sola fila. Con infohash es
certeza; sin él —que es lo normal, porque el hash vive en el magnet y el magnet
en la ficha— se apuesta por el parecido: misma obra, mismo año, mismo capítulo,
misma calidad y pesos que no se contradigan. Como es una apuesta, **el sitio que
se queda detrás sigue ahí, con su enlace**: si Imán se equivoca juntándolos, no
te ha escondido nada.

### Novedades

Buscar sirve cuando ya sabes qué quieres. `/novedades` es lo otro: entrar a ver
qué hay. Cada hora un rondín pasa por la sección de **películas** de cada sitio
—solo películas, y por la URL, no filtrando después—, apunta lo que no había
visto antes y se olvida de lo que lleva más de una semana. La página se sirve
siempre de lo apuntado: entrar en ella no dispara ni una petición a ningún
sitio. El botón «Actualizar» pide una ronda a mano y vuelve enseguida; la ronda
sigue por su cuenta.

Se puede ordenar de dos maneras: por lo último que apareció o por en cuántos
sitios está, que distingue el estreno que ha publicado todo el mundo del relleno
que solo tiene uno. La fecha que manda es **cuándo lo vio Imán**, no la del
sitio: solo uno de los tres la publica. Dentro de una misma ronda desempata el
puesto que ocupaba en la rejilla del sitio, que es el orden en el que él mismo
lo publicó.

Como las rejillas traen el título a medias y nunca el magnet, el rondín abre
unas cuantas fichas por ronda —las más nuevas primero— para completarlas. Eso es
lo que convierte «Admisin imposible» en «Admisión imposible», y de paso lo que
permite juntarla con la misma película de otro sitio.

DonTorrent no etiqueta el idioma en ninguna parte y DivxTotal solo lo etiqueta
en la ficha, que no se pide para pintar la lista. En los dos casos Imán **supone
castellano** cuando el título no dice otra cosa. Es su convención: lo que no es
castellano lo lleva escrito en el título, y lo comprobamos a mano antes de
confiar en ello.

## Desarrollo

```bash
go test ./...
go run ./cmd/iman        # http://localhost:8080
```

Los tests no tocan la red: los conectores se prueban contra capturas HTML en
`internal/conectores/testdata`. Para comprobar de verdad si un sitio sigue
sirviendo el mismo HTML, hay tests aparte que sí salen a internet:

```bash
go test -tags vivo -v ./internal/conectores/
```

Cuando esos fallen, es que el sitio ha cambiado y toca recapturar las fixtures.

Para ver al resolutor de dominios trabajar, se le miente sobre el dominio y se
mira el log: tiene que descartarlo y encontrar el bueno él solo.

```bash
echo '{"EliteTorrent":{"dominio":"https://www.elitetorrent.pl"}}' > /tmp/estado.json
IMAN_ESTADO=/tmp/estado.json go run ./cmd/iman
```

| Variable | Por defecto | Qué es |
|---|---|---|
| `IMAN_ADDR` | `:8080` | Dirección de escucha |
| `IMAN_VERSION` | la del build | Se muestra en `/salud` |
| `IMAN_ESTADO` | `/datos/estado.json` | Dónde se guardan los dominios vigentes |
| `IMAN_NOVEDADES` | `/datos/novedades.json` | Dónde se guarda la portada de novedades |
| `IMAN_TIEMPO_BUSQUEDA` | `20s` | Presupuesto de una búsqueda completa |
| `IMAN_RONDA_NOVEDADES` | `1h` | Cada cuánto se mira qué han subido los sitios |

## Despliegue

Push a `main` → CI construye y publica `ghcr.io/davic80/iman`. Detalles en
[DEPLOY.md](DEPLOY.md).

## Aviso

Imán es un **buscador**: no aloja contenido, no sirve `.torrent` propios y no
descarga nada. Hace lo mismo que Jackett, que lleva años siendo software libre
con miles de estrellas. Lo que hagas con los resultados es cosa tuya y de la ley
que te aplique.
