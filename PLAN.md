# Imán — buscador de torrents en castellano

Un buscador web que consulta en vivo los sitios de torrents españoles, filtra a
castellano de España de verdad, y te da el magnet o el `.torrent`.

Imán, porque eso es lo que devuelve.

---

## 1. Por qué existe esto

La respuesta corta: **porque Jackett y Prowlarr no cubren el contenido en
español, y no es por dejadez — es que no cabe en su formato.**

Eso no es una intuición. Es lo que salió de medirlo:

| Comprobación | Resultado |
|---|---|
| Definiciones de indexers en Prowlarr | 547 |
| Definiciones en Jackett | 551 |
| De esas, en español (`language: es-*`) | 14 |
| De esas 14, **privadas** (requieren cuenta) | 12 |
| Públicas y generalistas | **0** (la única pública es GamesTorrents, de videojuegos) |
| DonTorrent, MejorTorrent, DivxTotal, EliteTorrent, Wolfmax4k, PcTMix | **no existen** en ninguno de los dos |
| ¿Están en la blocklist de Prowlarr? | No. Simplemente nadie los mantiene. |
| Proyectos en GitHub que resuelvan esto | Ninguno serio. Scrapers sueltos de 0–9 estrellas, varios abandonados. |

Y el segundo dato, el que define el diseño entero. Cogí cinco películas grandes
(Dune 2, Oppenheimer, The Batman, Interstellar, Gladiator) y analicé los **490
resultados** que devuelve la API de The Pirate Bay:

| Etiqueta | Resultados |
|---|---|
| **Castellano explícito** | **0** |
| Dual (sin cualificar) | 11 |
| VOSE / subtitulado | 7 |
| Latino | 5 |
| "Español" / "Spanish" (ambiguo) | 5 |

Cero de 490. **Los sitios internacionales no son una fuente secundaria para este
proyecto: no son una fuente.** Cuando dicen "Spanish" casi siempre quieren decir
latino, y el criterio de este proyecto excluye el latino.

De ahí sale la conclusión incómoda pero clarificadora: **los sitios españoles no
son el diferenciador de Imán, son su única materia prima.** No hay una versión
fácil de este proyecto. El trabajo duro es la fase 1.

### Por qué no cabe en un YAML de Jackett

Esta es la definición española pública que sí existe, la de GamesTorrents:

```yaml
links:
  - https://www.gamestorrents.app/
legacylinks:
  - https://www.gamestorrents.com/
  - https://www.gamestorrents.tv/
  - https://www.gamestorrents.nu/
  - https://www.gamestorrents.fm/
```

Cinco dominios quemados, y **cada cambio lo arregla un humano mandando una pull
request.** Es una lista estática mantenida a mano.

Los sitios que nos interesan necesitan tres cosas que en ese YAML no se pueden
escribir:

1. Descubrir el dominio vigente **solos**, sin intervención humana.
2. Resolver un interstitial con JavaScript (DonTorrent) sin navegador headless.
3. Distinguir el sitio auténtico de un dominio parkeado que se hace pasar por él.

Las tres son código. Ahí está el proyecto.

---

## 2. Lo que verifiqué sobre el terreno

Probé 23 dominios el 1 de agosto de 2026. Esto es el estado real, no el que
dicen los listados:

**Muertos (NXDOMAIN):** `dontorrent.gs`, `dontorrent.forum`, `dontorrent.pro`,
`mejortorrent.one`, `elitetorrent.ec`, `grantorrent.pe`

**Vivos:** `divxtotal.tv`, `wolfmax4k.com` (Cloudflare), `esdocu.com`
(Cloudflare), `todotorrents.com` (openresty), `pctmix.com` (openresty),
`mejortorrentt.net`, `naranjatorrent.com`, `www.elitetorrent.wf` (WordPress
plano), `www43.mejortorrent.eu`

> **Revisión del 2026-08-05, cinco días después.** «Vivo» aguanta poco y
> «responde 200» no significa «sirve»:
>
> | Dominio | Entonces | Ahora |
> |---|---|---|
> | `www.elitetorrent.wf` | vivo | ✅ conectado |
> | `naranjatorrent.com` | vivo | ✅ es espejo de DonTorrent, ya entra como semilla |
> | `divxtotal.tv` | vivo | ❌ parking: 114 bytes, redirige a `/lander` |
> | `todotorrents.com`, `pctmix.com` | vivos | ❌ ya no resuelven |
> | `mejortorrentt.net` | vivo | ❌ ya no resuelve |
> | `www43.mejortorrent.eu` | vivo | ⛔ veta el ASN del servidor |
> | `wolfmax4k.com` | vivo (Cloudflare) | ⚠️ sin Cloudflare, pero inservible: ver abajo |
> | `esdocu.com` | vivo (Cloudflare) | ⚠️ sin Cloudflare, pero es de documentales y cursos, otro nicho |
>
> De nueve dominios «vivos», en cinco días quedan dos aprovechables y los dos ya
> están conectados. **La lista de candidatos hay que rehacerla, no consultarla.**

**Wolfmax4k no sirve, por dos razones independientes** (2026-08-05). Ninguna
tiene que ver con el anti-bot, que aquí no existe: contesta 200 con el HTML
entero y sin desafío.

1. **Su buscador siempre devuelve cero.** `POST /buscar` con su token CSRF y su
   cookie, `GET /buscar/<q>`, `?q=`, con `Referer` y sin él: todos contestan 200
   con «Resultados Encontrados para : x (  )» y el contador vacío. Y no es que no
   tenga el título: buscando «dink» da cero teniendo *Dink (The Dink) (2026)* en
   portada. El índice no responde desde fuera.
2. **No publica ni magnet ni `.torrent`.** El único botón de descarga apunta a
   `enlacito.com/s.php?i=<base64>`, un acortador monetizado. En toda la ficha y
   toda la portada: cero apariciones de `magnet:` y cero de `.torrent`.

La segunda razón sola ya lo descarta aunque el buscador se arreglase: Imán
promete darte el enlace, no mandarte a una cadena de anuncios.

### Inventario rehecho (2026-08-06)

43 dominios sondeados desde el servidor, y esta vez el criterio no es «responde
200» sino los tres requisitos de verdad: **que se le pueda buscar desde el
servidor, que sirva el enlace él mismo, y que tenga castellano.**

**Aptos — dos:**

| Sitio | Buscar | Enlace | Castellano |
|---|---|---|---|
| **DivxTotal** (`divxtotal.foo`) | `?s=<q>`, WordPress, sin anti-bot | `.torrent` propio vía `download_tt.php?u=<base64>` | sí, `inLanguage: es-ES` |
| **Knaben** (`api.knaben.org/v1`) | API JSON, sin scraping | `magnetUrl` + `hash` | poco, ver abajo |

**DivxTotal ha resucitado.** `divxtotal.tv` es un parking, pero el sitio se mudó
a `divxtotal.foo` y está vivo y actualizado (subidas hasta julio de 2026).
Descargado un `.torrent` suyo: 104.577 bytes, bencode válido, servido desde su
propio dominio. El `download_tt.php?u=<base64>` parece un acortador y no lo es —
el base64 decodifica a una URL suya. Es la diferencia con Wolfmax4k.

**Knaben es otra cosa y hay que decidirlo aparte.** Es un meta-indexador con API
JSON sobre trackers internacionales (TPB, 1337x, Nyaa, RuTracker), y es el único
candidato que da **semillas y tamaño**, que es justo lo que a DonTorrent le
falta. Pero su castellano es fino. Medido sobre ocho consultas típicas:

| Consulta | Resultados | En castellano | Con semillas |
|---|---|---|---|
| el padrino | 67 | 11 | 4 |
| la casa de papel | 119 | 4 | 2 |
| torrente | 156 | 27 | 15 |
| ocho apellidos vascos | 18 | 5 | 3 |
| los otros | 24 | 11 | 3 |
| as bestas | 6 | 1 | 1 |
| el hoyo | 29 | 0 | 0 |
| campeones | 51 | 2 | 1 |

Cuatro resultados vivos para «el padrino», cuando DonTorrent y EliteTorrent dan
46 entre los dos. Aporta poco en cantidad; aporta semillas, que no es poco.

**Descartados, con el motivo:**

| Sitio | Motivo |
|---|---|
| `torrentazos.com` | «Nothing found» para todas las consultas. Índice muerto |
| `elitetorrent.one` | **Cáscara**: devuelve la misma página byte a byte para cualquier ruta, incluida una inventada. Portadas servidas desde `yts.mx` y marcas de idioma `fr`. Usa la marca EliteTorrent sin ser EliteTorrent |
| `1337x.to`, `ext.to`, `torrentgalaxy.one` | Desafío de Cloudflare (403) desde el servidor |
| `thepiratebay.org` | Responde 5 KB, cáscara de JS. Y Knaben ya lo indexa |
| `nyaa.si` | Anime, sin castellano |
| `bitsearch.eu` (= `solidtorrents.to`) | Mismo nicho que Knaben. Queda de recambio |
| `esdocu.com` | Documentales y cursos: otro nicho |
| Familia `descargas2020` / `pctnew` / `pctreload` / `pctfenix` / `torrentlocura` / `tumejortorrent` / `torrentrapid` / `grantorrent` | Muertos: ni DNS ni respuesta. Los ocho |
| `yts.mx`, `torlock.com`, `atomohd.blog`, `elitetorrent.ec`, `elitetorrent.biz`, `divxtotal1.com` | Sin DNS o sin respuesta |

Lo que enseña el inventario: **de 43 dominios sale un solo conector nuevo.** No
es un mal día de búsqueda, es el estado del terreno. Conviene contar con que la
fase 3 sea corta y que el mantenimiento de los dos que ya funcionan valga más
que buscar el tercero.

**Trampa detectada:** `dontorrent.click` responde 200 y parece el sitio, pero es
un **dominio parkeado en GoDaddy** que sirve anuncios y un iframe a
`yfdpco1.com`. Un scraper ingenuo se lo tragaría y te serviría basura. De aquí
sale el requisito de verificación de autenticidad.

**Tres vectores de descubrimiento de dominio, todos confirmados:**

1. Los dominios viejos hacen 301 al nuevo: `mejortorrent.wtf` → `www43.mejortorrent.eu`
2. Los canales de Telegram publican el dominio vigente en HTML público sin auth:
   `t.me/s/MejorTorrentAp` me dio el `www43` correcto
3. Contador numérico incremental en el subdominio: `www43.` sugiere que hubo un
   `www42.` y habrá un `www44.`

**El anti-bot es más blando de lo que parece.** DonTorrent monta un interstitial
con FingerprintJS, pero el propio HTML filtra el bypass en el `<noscript>`:

```
GET /buscar/matrix
  → HTML con  var redirect_link = 'http://.../buscar/matrix?tr_uuid=<uuid>&'
  → repites con  redirect_link + "fp=-5"
  → contenido real
```

Sin navegador headless. Verificado: la segunda petición pasó de 1.1 KB a 17 KB.

> **Corrección del 2026-08-05, al construir el conector.** Ese interstitial no
> era de DonTorrent: **era del parking**. Siguiendo el bypass desde
> `dontorrent.click` se aterriza en `ww38.dontorrent.click`, con "¡Este dominio
> podría estar en venta!", `godaddy` y `sk-park` en el HTML. Los 17 KB eran su
> página de anuncios. La lista de `MarcasDeParking` lo caza sin cambios.
>
> La puerta real del sitio es mucho más tonta: **comprueba el `Referer`**. Sin
> él contesta 200 con "Necesitas utilizar el buscador" y cero resultados. Con
> él, resultados. Aislado y verificado.
>
> Y una trampa para el futuro: la palabra "dontorrent" aparece **cero** veces
> en el HTML de los espejos buenos y **cinco** en el del parking. Usarla como
> marca positiva de la huella premiaría al impostor.

**Rate limiting real.** EliteTorrent me respondía y, tras unas cuantas
peticiones seguidas, empezó a dar timeout. Hay que ir con cuidado: límite por
host, backoff y circuit breaker no son adorno.

**Hay sitios que vetan el ASN de Hetzner** (descubierto el 2026-08-05). No es un
desafío que se resuelva: Cloudflare devuelve `error code: 1005` a cualquier
petición desde el rango del servidor, y llega **antes** de que haya JavaScript
que ejecutar, así que FlareSolverr no sirve de nada aquí.

| Dominio | Desde Hetzner |
|---|---|
| `dontorrent.management` (el que el sitio anuncia como oficial) | 403, `error code: 1005` |
| `www43.mejortorrent.eu` | 403, `error code: 1005` |
| `donproxies.com` (su proxy oficial) | 403 |

La salida no es pelearse con el bloqueo, es **usar otro dominio del mismo
sitio**: TomaDivx y NaranjaTorrent sirven el catálogo de DonTorrent y no vetan a
nadie. Encaja con el resolutor sin tocarlo: los dominios vetados se quedan como
semillas, fallan la verificación y se pasa al siguiente.

Lo que sí tumbaría esto es que **todos** los espejos vetaran el ASN a la vez.
Entonces harían falta peticiones desde una IP residencial española, y eso es
hardware en casa, no código.

**Y eso es exactamente lo que pasa con MejorTorrent** (comprobado el
2026-08-05). Buscado a fondo y no hay espejo bueno:

- `www44`/`www45`, `mejortorrent.eu`, `.one`, `mejortorrentt.net` → no resuelven
- `mejortorrent.org`, `mejortorrent1.com` → responden 200, pero son parkings de
  `abovedomains` ("This domain may be for sale"), 1 KB de HTML
- `t.me/s/MejorTorrentAp`, su canal oficial, sigue anunciando **solo** `www43`
- `www43` da 1005 por IPv4 **y por IPv6**: el veto es del ASN entero

El propio sitio recomienda usar Cloudflare WARP cuando falla el acceso, que es
justo el síntoma de esto. Y los dominios parecidos que aparecen al buscar son
impostores, no espejos: es la situación que la huella está hecha para rechazar.

**Decisión: MejorTorrent queda fuera hasta que haya una IP residencial
española.** No es un problema de parser — el parser no serviría de nada porque
desde producción no se llega al HTML. Se salta a la fase 3.

**Desde España hay censura de DNS.** El resolutor del ISP devuelve `0.0.0.0`
para estos dominios; contra 1.1.1.1 resuelven todos. Al servidor de Alemania no
le afecta, pero al desarrollar desde casa engaña: parece que el sitio está
muerto cuando lo que está muerto es la respuesta del DNS.

**El servidor.** `46.225.211.9` es Hetzner, Núremberg. No está en España, así
que los bloqueos por orden judicial a ISPs españoles no aplican.

---

## 3. Decisiones tomadas

| # | Decisión | Elegido |
|---|---|---|
| 1 | Qué es | Webapp de búsqueda. Devuelve magnet + `.torrent` |
| 2 | Exposición | Repo público, instancia con basic auth y `noindex` |
| 3 | Idioma | Castellano de España, estricto. Sin latino, sin VOSE. Dual solo si se puede cualificar |
| 4 | Trackers privados | Fuera del camino crítico. Conectores con credenciales opcionales |
| 5 | Contenido | Cine y series primero |
| 6 | Estrategia | Búsqueda en vivo con caché corta. Sin base de datos |
| 7 | Lenguaje | **Go** |
| 8 | Frontend | SSR con plantillas + htmx |
| 9 | Metadatos | TMDB (carátulas, agrupar calidades por ficha) |
| 10 | Nombre | **Imán** (repo y dominio: `iman`, sin tilde) |

Y lo que **no** es, para que quede escrito:

- No es un proxy Torznab para Sonarr/Radarr. El modelo de datos será compatible
  para que añadirlo después sea barato, pero no se construye ahora.
- No es un crawler de DHT. Nada de indexar la red entera.
- No aloja contenido ni `.torrent`. Es un buscador.

---

## 4. Arquitectura

Un solo contenedor. Sin base de datos. El estado que hay que conservar entre
reinicios (qué dominio funciona) cabe en un JSON.

```
navegador ──► Caddy (TLS + basic auth) ──► imán (Go, :8080)
                                              │
              ┌───────────────────────────────┼───────────────────────────┐
              ▼                               ▼                           ▼
      resolutor de dominios          motor de fan-out              enriquecedor
      (quién está vivo hoy)          (N conectores en             (TMDB: carátulas,
              │                       paralelo, con                agrupar por obra)
              │                       timeouts)
              ▼                               │
       estado.json (volumen)                  ▼
                                    clasificador de idioma
                                    y calidad ← el filtro castellano
                                              │
                                              ▼
                                    deduplicador (por infohash)
```

Siete piezas:

1. **Resolutor de dominios** — averigua qué dominio está vivo hoy y verifica que
   es auténtico
2. **Conectores** — uno por sitio, cada uno sabe buscar y parsear el suyo
3. **Motor de fan-out** — lanza los conectores en paralelo con presupuesto de
   tiempo, aísla fallos
4. **Clasificador** — extrae idioma, calidad, año, temporada/episodio del título
5. **Deduplicador** — por infohash, y agrupa por obra
6. **Enriquecedor TMDB** — carátula y sinopsis
7. **UI** — plantillas Go + htmx

---

## 5. El resolutor de dominios

Es la pieza que justifica el proyecto, así que va con detalle.

El problema: los sitios cambian de dominio cada pocas semanas y a veces el
dominio abandonado lo compra un parking que se hace pasar por ellos.

### Cascada de estrategias

Para cada sitio, en orden, hasta que una funcione:

1. **Último dominio bueno conocido** (desde `estado.json`). El caso normal.
2. **Semillas de configuración** — lista de candidatos que mantenemos en el
   conector.
3. **Seguir redirecciones** desde dominios viejos conocidos. Verificado:
   `mejortorrent.wtf` → `www43.mejortorrent.eu`.
4. **Canal de Telegram** — `GET https://t.me/s/<canal>`, extraer dominios del
   HTML. Es público y no requiere auth. Verificado.
5. **Contador incremental** — si el dominio vigente es `www43.sitio.eu`, probar
   `www44`, `www45`… Barato y cubre el patrón de MejorTorrent.

### Verificación de autenticidad (obligatoria)

Ningún dominio se acepta por responder 200. Tiene que pasar las tres:

```go
type SiteFingerprint struct {
    MustContain    []string // marcadores del sitio real: título, textos, clases CSS
    MustNotContain []string // marcadores de parking
    ProbeQuery     string   // búsqueda de prueba, p.ej. "matrix"
    MinProbeHits   int      // resultados parseables mínimos
}
```

1. **Marcadores positivos**: el HTML contiene lo que ese sitio contiene siempre.
2. **Marcadores negativos**: nada de `godaddy`, `buy this domain`,
   `sedoparking`, `sk-park.php`, `"mode":"iframe"`, `Comprar este dominio`.
   Esta lista sale directamente de diseccionar `dontorrent.click`.
3. **Prueba funcional**: una búsqueda conocida devuelve al menos N resultados
   que el parser entiende. Es la única verificación que no se puede falsear:
   un parking no sabe buscar "matrix".

### Ciclo de vida

- Revalidación en segundo plano cada N horas, nunca en el camino de una búsqueda
  del usuario.
- Cuando un dominio falla, se marca y se dispara la cascada. Mientras tanto ese
  conector queda fuera y los demás siguen respondiendo.
- `estado.json` en un volumen, para no empezar de cero en cada despliegue.
- Todo cambio de dominio se registra en el log. Es la señal de que algo se movió.

---

## 6. Conectores

```go
type Connector interface {
    ID() string
    Name() string
    Fingerprint() SiteFingerprint
    SeedDomains() []string
    TelegramChannel() string          // "" si no tiene
    Search(ctx context.Context, base string, q Query) ([]Result, error)
    FetchTorrent(ctx context.Context, base, ref string) ([]byte, error)
}
```

Nada de reflexión ni registros mágicos: un slice de conectores construido en
`main`. Añadir un sitio es escribir un fichero y una línea.

### Orden de ataque

| Fase | Sitio | Dificultad | Notas |
|---|---|---|---|
| 1 | **EliteTorrent** (`elitetorrent.pl`) | Baja | ✅ Hecho. WordPress plano, `?s=<query>` |
| 1 | **DivxTotal** (`divxtotal.foo`) | Baja | ✅ Hecho. `.tv` es parking, pero el sitio se mudó al `.foo`. El enlace bueno va en un `data-src` en base64 |
| 1 | ~~**TodoTorrents / PcTMix**~~ | — | ❌ Muertos: ya no resuelven |
| 2 | **DonTorrent** (`tomadivx.net`) | Media | ✅ Hecho. La puerta era el `Referer`, no el interstitial |
| 2 | ~~**MejorTorrent**~~ | — | ⛔ Bloqueado: su único dominio veta el ASN y no tiene espejos. Ver §2 |
| 2 | ~~**Naranjatorrent**~~ | — | Es el mismo sitio que DonTorrent: ya entra como semilla suya |
| 3 | ~~**Wolfmax4k**~~ | — | ❌ Buscador siempre a cero y descarga por acortador. Ver §2 |
| 3 | **Esdocu** | Baja | Responde sin Cloudflare, pero es documentales y cursos: otro nicho |
| 3 | **Knaben** (`api.knaben.org/v1`) | Baja | API JSON, única fuente con semillas. Castellano fino: decidir aparte |
| 4 | Privados (HD-Olimpo, Torrenteros…) | — | Solo si algún día hay cuenta |

Dos correcciones de la tabla original, ambas del 2026-08-05 y ambas del mismo
tipo: **lo que se ve desde el navegador de casa no es lo que ve el servidor.**
DivxTotal parecía vivo y es un parking; Naranjatorrent parecía un sitio nuevo y
es un espejo de uno que ya teníamos.

El primer conector se construye entero (parser, tests, verificación) antes de
tocar el segundo. Es el que define la forma de la interfaz; hacer tres a medias
en paralelo es cómo se acaba con tres parsers incompatibles.

### Tests contra HTML congelado

Los selectores se rompen. La única defensa razonable: guardar respuestas HTML
reales en `testdata/` y tener tests que parseen esos ficheros. Cuando un sitio
cambie el maquetado, se descarga el HTML nuevo, se ve el diff y se arregla el
parser con un test que lo demuestra. Sin red en los tests, para que el CI no
dependa de que los sitios estén vivos.

---

## 7. El filtro de castellano

El criterio es filtro duro, no ordenación. Lo que no sea castellano de España no
se muestra.

### Vocabulario

| Categoría | Etiquetas | Acción |
|---|---|---|
| **Aceptar** | `Castellano`, `Cast`, `ESP`, `ES-ES`, `Español (España)` | Pasa |
| **Rechazar** | `Latino`, `LAT`, `ES-LA`, `VOSE`, `VOSI`, `Subtitulado`, `Sub Esp` | Fuera |
| **Ambiguo** | `Español`, `Spanish`, `Dual`, `Multi` | Resolver por contexto |

### Las dos reglas que importan

**"Español" a secas no es castellano.** En fuente internacional tiende a
significar latino. Se trata como *desconocido*, nunca como aceptado.

**"Dual" hay que cualificarlo.** En un sitio español, Dual suele ser
castellano+inglés (vale). En fuente internacional o latina, suele ser
latino+inglés (no vale). Regla:

```
aceptar Dual  ⟺  hay marca ES-ES explícita
              ∨  la fuente es nativamente castellana (sitio español)
```

Cada conector declara si su fuente es nativamente castellana. Eso resuelve el
ambiguo sin adivinar.

### Y de paso, del título salen

Calidad (`4K`, `2160p`, `1080p`, `720p`, `BluRay`, `WEB-DL`, `HDTV`, `DVDRip`,
`CAM`), año, temporada y episodio (`S01E03`, `1x03`, `Temporada 2`), códec
(`x265`, `HEVC`, `H264`) y HDR/DV. Un solo parser de títulos, muy testeado, que
alimenta tanto el filtro como los filtros de la UI.

---

## 8. Modelo de datos

```go
type Result struct {
    Title     string
    Source    string        // id del conector
    InfoHash  string        // clave de deduplicación
    Magnet    string
    TorrentRef string       // ref opaca para que el servidor lo descargue
    SizeBytes int64
    Seeders   int
    Leechers  int
    Published time.Time

    Language  Language      // Castellano | Dual | Desconocido | Rechazado
    Quality   Quality
    Year      int
    Season    int
    Episode   int

    TMDBID    int           // 0 si no hay match
}
```

Campos deliberadamente cercanos a Torznab (`InfoHash`, `SizeBytes`, `Seeders`,
`Published`) para que el día que quieras el endpoint sea traducción, no rediseño.

**Deduplicación por `InfoHash`.** El mismo torrent aparece en varios sitios; se
funde en una entrada que recuerda de dónde vino. Cuando no hay infohash (algunos
sitios solo dan `.torrent`), se cae a una clave normalizada de título+tamaño.

**Agrupación por obra** vía TMDB: una ficha de "Dune: Parte Dos" con sus
calidades dentro, en vez de doce filas sueltas.

---

## 9. Interfaz

Plantillas de Go + htmx. Sin build de JavaScript, sin `node_modules`, sin SPA.
La app es una caja de búsqueda y una lista: SSR es exactamente la herramienta.

- **Resultados en streaming.** El fan-out tarda lo que tarde el sitio más lento.
  Con htmx se pintan los resultados según llegan en vez de esperar al último.
  Un conector caído no bloquea la página.
- **Filtros** de calidad, tamaño, seeders y fuente, en la propia URL para que
  sean compartibles.
- **Botón de copiar magnet** y **descarga de `.torrent`**.
- **`/salud`** — tabla con cada conector: dominio en uso, última verificación,
  latencia, tasa de error. Dado lo frágil que es esto por naturaleza, saber *qué*
  se ha roto y *cuándo* no es un lujo.

### El `.torrent` lo sirve el servidor

Nunca se enlaza directamente al sitio origen. El navegador pide
`/torrent/<fuente>/<ref>` y Go lo descarga y lo reenvía. Tres razones: muchos
sitios exigen cookies o haber pasado el interstitial, así se evita filtrar al
usuario contra el sitio origen, y así el navegador no acaba en una página llena
de anuncios.

---

## 10. qBittorrent — descartado

> **Cerrado el 2026-08-05: no se hace.** El qBittorrent de David vive en una
> raspberry de su LAN, junto al disco de 10 TB, y se accede por
> `torrent.ojoalprecio.com` con Cloudflare Access y autenticación por correo.
> Imán vive en el Hetzner. Conectarlos era posible (service token de Access, o
> Tailscale entre las dos máquinas) pero **David prefiere copiar el magnet a
> mano**, que es lo que hace hoy y le vale.
>
> Tampoco se mueve Imán a la raspberry: Imán toca qBittorrent una vez por
> descarga, con un POST al que no le importa la latencia. Eso es acoplamiento
> flojo y no justifica una mudanza. Lo que Imán hace todo el rato es rascar
> sitios y servir HTML.
>
> Lo de abajo se queda como apunte por si algún día cambia la topología.

**No es difícil.** Tiene API HTTP documentada y son dos peticiones:

```
POST /api/v2/auth/login     username, password  +  header Referer
    → Set-Cookie: SID=...
POST /api/v2/torrents/add   multipart, urls=magnet:...  +  Cookie: SID=...
```

Lo difícil no es la API, es **dónde vive tu qBittorrent**. Aquí necesito un dato
tuyo:

| Caso | Solución |
|---|---|
| **a) qBittorrent en el mismo Hetzner** | Trivial. Se hablan por la red Docker. Cero fricción |
| **b) Expuesto en internet tras tu Caddy** | Igual de fácil, servidor a servidor |
| **c) Solo en tu red local** | Aquí está el problema |

En el caso (c), el servidor de Hetzner no llega a tu LAN. La alternativa
—llamar a qBittorrent desde el navegador, que sí está en tu red— choca con dos
muros: qBittorrent valida `Referer`/`Origin` contra su propio Host, y sobre todo
una página servida por `https://iman.…` no puede hacer peticiones a
`http://192.168.x.x:8080` porque el navegador lo bloquea por contenido mixto.
Se puede forzar desactivando protecciones de qBittorrent, pero es debilitar su
seguridad para ganar comodidad, y no lo recomiendo.

Si estás en el caso (c), lo limpio es un túnel (Tailscale o WireGuard) entre el
servidor y la máquina de qBittorrent. Con eso vuelves al caso (a).

**Decisión de diseño:** la función se construye opcional y desactivada por
defecto, configurada por variables de entorno (`QBITTORRENT_URL`, `_USER`,
`_PASS`). Si no están, el botón no aparece. El proyecto no depende de esto.

---

## 11. Comportarse bien (y no acabar bloqueado)

Ya me limitaron el ritmo durante la investigación, así que esto va en el diseño
desde el principio:

- **Límite de peticiones por host**, no global. Cada sitio es un vecino distinto.
- **Backoff exponencial con jitter** ante errores y 429.
- **Circuit breaker**: N fallos seguidos y el conector se aparta un rato. Una
  búsqueda no debe esperar a un sitio que está caído.
- **Caché de resultados** con TTL corto (minutos). Dos búsquedas iguales
  seguidas no son dos rondas de scraping.
- **Presupuesto de tiempo global** por búsqueda. Lo que no llegue, no llega; se
  muestra lo que hay.
- **Cabeceras de navegador realista** y reutilización de cookies por sitio.
- **Concurrencia limitada por sitio.** Son webs pequeñas; machacarlas es de mala
  educación y además te gana un baneo.

---

## 12. Estructura del repositorio

```
iman/
├── cmd/iman/main.go          ✓ servidor + modo -sonda para el HEALTHCHECK
├── recursos.go               ✓ //go:embed de plantillas y estáticos
├── internal/
│   ├── web/                  ✓ handlers, plantillas, htmx
│   ├── conectores/             uno por sitio + interfaz + testdata/
│   ├── dominios/               resolutor, verificación, estado.json
│   ├── buscador/               fan-out, timeouts, circuit breaker, caché
│   ├── titulos/                parser de idioma, calidad, temporada
│   └── tmdb/
├── plantillas/               ✓
├── estaticos/                ✓
├── Dockerfile                ✓
├── docker-compose.yml        ✓
├── .github/workflows/ci.yml  ✓
├── DEPLOY.md                 ✓
└── README.md                 ✓
```

Los paquetes sin marcar todavía no existen: se crean cuando tengan contenido, no
antes. `recursos.go` vive en la raíz porque `//go:embed` no puede subir de
directorio, y las plantillas van en la raíz por comodidad al editarlas.

---

## 13. Despliegue

Calcado de gorilla, que es lo que ya funciona.

**Dockerfile** multi-etapa: `golang:1.23-alpine` compila un binario estático, y
la imagen final es `gcr.io/distroless/static` con el binario dentro (plantillas
y estáticos van empotrados con `//go:embed`). Medido en la fase 0: **3,9 MB
comprimida**, arranque instantáneo, sin shell dentro.

**CI** (`.github/workflows/ci.yml`), misma forma que gorilla:

```
job test    → go vet, go test ./..., go build
job docker  → solo en push a main, needs: test
              buildx + login ghcr + push
              tags: ghcr.io/davic80/iman:latest
                    ghcr.io/davic80/iman:${{ github.sha }}
              cache: type=gha
```

**docker-compose.yml** con el mismo patrón: se engancha a la red externa
`cloud_default`, solo `expose` interno, alias `iman`, logging rotado. Añade un
volumen para `estado.json`, que gorilla no necesitaba.

**Caddy** — al Caddyfile compartido de `~/padelscores/cloud`:

```
iman.ojoalprecio.com {
    basic_auth {
        david <hash-bcrypt>
    }
    reverse_proxy iman:8080
}
```

Y `docker exec cloud-caddy-1 caddy reload --config /etc/caddy/Caddyfile`.

**Cloudflare**: registro A a `46.225.211.9` en **nube gris**. Con la naranja,
Cloudflare intercepta el desafío HTTP-01 y Caddy no emite el certificado. Está
escrito en tu propio DEPLOY.md de gorilla y merece repetirse.

**Vuelta atrás**: `IMAN_TAG=<sha> docker compose up -d`.

---

## 14. Fases

> **Dónde vamos:** fases 0, 1 y 2 hechas. La 2 se cerró con el resolutor entero
> (cascada, verificación, `estado.json`, `/salud`) y **DonTorrent**, los dos
> verificados en vivo; MejorTorrent, que era lo que quedaba, no se alcanza desde
> el servidor y queda aparcado. En la **fase 3** está hecho **DivxTotal**, el
> tercer conector y única cosecha del inventario rehecho. Queda decidir Knaben y
> la deduplicación por infohash.

**Fase 0 — el tubo entero, vacío.** Repo, `hello world` en Go, Dockerfile, CI,
compose, Caddy, DNS. Objetivo: ver `iman.ojoalprecio.com` pidiendo contraseña y
respondiendo. Validar el despliegue *antes* de que haya nada que desplegar es lo
que evita depurar dos problemas a la vez.

**Fase 1 — vertical completa con un sitio.** EliteTorrent entero: conector,
parser de títulos, filtro de castellano, UI de búsqueda, magnet y `.torrent`
proxiado. Al final de esta fase la app ya sirve para algo.

**Fase 2 — el resolutor de dominios.** La cascada, la verificación de
autenticidad, `estado.json`, `/salud`. Y con eso montado, DonTorrent, que es el
que lo necesita. MejorTorrent iba aquí y se cae: no hay dominio suyo al que el
servidor pueda llegar.

**Fase 3 — anchura.** Inventario rehecho el 6 de agosto: de 43 dominios sale
**DivxTotal (`divxtotal.foo`)**, y ese es el tercer conector. Knaben queda como
decisión aparte, porque no es más de lo mismo: es una API con semillas y poco
castellano. Con tres sitios, deduplicación por infohash.

**Fase 4 — acabado.** TMDB, agrupación por obra, filtros en la UI.

**Fase 5 — opcionales.** Cloudflare vía FlareSolverr, endpoint Torznab si
alguna vez montas un *arr. (qBittorrent estaba aquí y está descartado: ver la
sección 10.)

---

## 15. Riesgos

| Riesgo | Mitigación |
|---|---|
| Los selectores se rompen | Tests con HTML congelado + `/salud` que lo canta |
| Un sitio muere del todo | Los conectores están aislados; los demás siguen |
| Dominio parkeado suplantando | Verificación por huella + prueba funcional |
| Baneo por ritmo | Límites por host, backoff, circuit breaker, caché |
| Cloudflare se extiende a más sitios | FlareSolverr como contenedor opcional (fase 5) |
| Pocos resultados por filtro estricto | Es el objetivo. Precisión antes que volumen. Si molesta, el filtro se relaja con un conmutador |

Y uno que no es técnico y toca decir una vez: el repo es público bajo tu nombre
y la instancia vive en Hetzner (Alemania). La instancia va con auth y `noindex`,
que fue tu decisión y es la sensata. El código en sí es un scraper genérico —
Jackett lleva años siendo público con 13k estrellas haciendo exactamente esto.

---

## 16. Lo primero que haría

Fase 0 completa y desplegada, y en cuanto esté verde, el conector de
EliteTorrent con sus tests. Ese conector es el que fija la forma de la interfaz
`Connector`, y prefiero descubrir que la interfaz está mal con uno escrito que
con cinco.
