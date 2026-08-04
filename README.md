# Imán 🧲

Buscador de torrents **en castellano de España**. Consulta en vivo varios sitios
españoles, filtra el idioma de verdad y te da el magnet o el `.torrent`.

Sin latino. Sin VOSE. Sin resultados que dicen "Spanish" y no lo son.

> **Estado: fase 2.** Funciona de punta a punta con un solo sitio
> (EliteTorrent): buscas, filtra el idioma, ordena y te da el magnet o el
> `.torrent`. El resolutor de dominios ya está montado, así que cuando el sitio
> se mude Imán lo encontrará solo. Faltan los demás conectores.
> Ver [PLAN.md](PLAN.md).

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
| Deduplicador | Funde por infohash y agrupa por obra |

Sin base de datos. Un solo contenedor. Plantillas de Go y htmx, sin build de
JavaScript.

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
| `IMAN_TIEMPO_BUSQUEDA` | `20s` | Presupuesto de una búsqueda completa |

## Despliegue

Push a `main` → CI construye y publica `ghcr.io/davic80/iman`. Detalles en
[DEPLOY.md](DEPLOY.md).

## Aviso

Imán es un **buscador**: no aloja contenido, no sirve `.torrent` propios y no
descarga nada. Hace lo mismo que Jackett, que lleva años siendo software libre
con miles de estrellas. Lo que hagas con los resultados es cosa tuya y de la ley
que te aplique.
