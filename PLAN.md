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

**Rate limiting real.** EliteTorrent me respondía y, tras unas cuantas
peticiones seguidas, empezó a dar timeout. Hay que ir con cuidado: límite por
host, backoff y circuit breaker no son adorno.

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
| 1 | **EliteTorrent** (`elitetorrent.wf`) | Baja | WordPress plano, `?s=<query>`. Verificado: devuelve "Resultados de matrix" |
| 1 | **DivxTotal** (`divxtotal.tv`) | Baja | Vivo, sin Cloudflare |
| 1 | **TodoTorrents / PcTMix** | Baja | openresty, vivos |
| 2 | **DonTorrent** | Media | Interstitial FingerprintJS. Bypass ya resuelto |
| 2 | **MejorTorrent** | Media | Contador `wwwNN.` + canal de Telegram |
| 2 | **Naranjatorrent** | Baja | Vivo, por explorar |
| 3 | **Wolfmax4k**, **Esdocu** | Alta | Cloudflare de verdad. Puede requerir FlareSolverr |
| 4 | Privados (HD-Olimpo, Torrenteros…) | — | Solo si algún día hay cuenta |

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

## 10. qBittorrent

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

**Fase 0 — el tubo entero, vacío.** Repo, `hello world` en Go, Dockerfile, CI,
compose, Caddy, DNS. Objetivo: ver `iman.ojoalprecio.com` pidiendo contraseña y
respondiendo. Validar el despliegue *antes* de que haya nada que desplegar es lo
que evita depurar dos problemas a la vez.

**Fase 1 — vertical completa con un sitio.** EliteTorrent entero: conector,
parser de títulos, filtro de castellano, UI de búsqueda, magnet y `.torrent`
proxiado. Al final de esta fase la app ya sirve para algo.

**Fase 2 — el resolutor de dominios.** La cascada, la verificación de
autenticidad, `estado.json`, `/salud`. Y con eso montado, DonTorrent (con su
bypass) y MejorTorrent (con Telegram), que son los que lo necesitan.

**Fase 3 — anchura.** DivxTotal, TodoTorrents/PcTMix, Naranjatorrent.
Deduplicación por infohash con varios sitios de verdad.

**Fase 4 — acabado.** TMDB, agrupación por obra, filtros en la UI.

**Fase 5 — opcionales.** qBittorrent (cuando me digas la topología), Cloudflare
vía FlareSolverr, endpoint Torznab si alguna vez montas un *arr.

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
