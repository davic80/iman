# Despliegue

Imán es un binario Go en una imagen distroless. El único estado es un
`estado.json` con el dominio vigente de cada sitio, y perderlo no rompe nada:
solo obliga a redescubrirlos en el siguiente arranque. Volver a una versión
anterior es cambiar un tag y levantar de nuevo.

## Cómo encaja en el servidor

```
Cloudflare (DNS only, nube gris)
        │
        ▼
  cloud-caddy-1  ← termina TLS (Let's Encrypt), y enruta
        │  red docker: cloud_default
        ▼
      imán       ← :8080, pide cuenta de Google; solo alcanzable desde dentro
```

El Caddy compartido vive en `~/padelscores/cloud`. Imán es un proyecto compose
aparte que se engancha a su misma red, igual que `gorilla`, `gasolineras`,
`reeldown` y `pixelface`.

## Publicar una versión

Cada push a `main` dispara CI: formato, `go vet`, tests con `-race`, build y
publicación de `ghcr.io/davic80/iman:latest` junto a un tag con el SHA del
commit. No hay que construir nada a mano.

El SHA viaja dentro del binario: se ve en `/salud` y así se sabe exactamente qué
versión está corriendo sin entrar al servidor.

## Primera instalación en el servidor

```bash
ssh david@46.225.211.9
mkdir -p ~/iman && cd ~/iman
```

Deja ahí `docker-compose.yml` y un `.env`:

```bash
PROXY_NETWORK=cloud_default
IMAN_TAG=latest
# Carátulas. Opcional: sin esto Imán arranca igual y no habla con TMDB.
IMAN_TMDB=<el API Read Access Token de themoviedb.org>
```

`IMAN_TMDB` es lo único secreto que hay aquí, y por eso vive solo en este
fichero: no está en `.env.example` ni en el repo. Se pide en
[themoviedb.org/settings/api](https://www.themoviedb.org/settings/api), tipo
*Developer*, y es gratis para uso no comercial. Después de tocarlo hace falta
`docker compose up -d` para que el contenedor lo lea. Para comprobar que ha
entrado, el log del arranque dice `"carátulas" tmdb=true`.

Autentica contra GHCR una sola vez y arranca:

```bash
docker compose up -d
```

## Acceso con Google

La instancia es privada y la autenticación la pone la propia app: entra con
Google y solo deja pasar a `IMAN_PERMITIDO` (por defecto
`david.cornejo@gmail.com`). `/vivo` queda libre para el healthcheck.

1. En [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
   crea un *ID de cliente de OAuth* de tipo **Aplicación web**, con URI de
   redirección autorizada `https://iman.ojoalprecio.com/oauth/google`. La
   pantalla de consentimiento puede quedarse en modo *Testing* con tu correo
   como usuario de prueba.
2. Añade al `.env` del servidor:

```bash
IMAN_GOOGLE_ID=<client id>.apps.googleusercontent.com
IMAN_GOOGLE_SECRETO=<client secret>
IMAN_CLAVE_SESION=<openssl rand -base64 32>
```

Sin `IMAN_CLAVE_SESION` funciona igual, pero cada despliegue cierra la sesión.
Sin `IMAN_GOOGLE_ID` la app arranca **abierta** y lo avisa en el log
(`acceso abierto`). Con todo puesto, el log dice `"acceso con google"`.

El hostname en el Caddyfile compartido (`~/padelscores/cloud/Caddyfile`) ya no
lleva `basic_auth`:

```
iman.ojoalprecio.com {
    reverse_proxy iman:8080
}
```

Recarga Caddy sin cortar el resto de sitios:

```bash
docker exec cloud-caddy-1 caddy reload --config /etc/caddy/Caddyfile
```

En Cloudflare hace falta un registro `A` apuntando a `46.225.211.9` **en modo
DNS only (nube gris)**. Con la nube naranja, Cloudflare intercepta el desafío
HTTP-01 y Caddy no consigue emitir el certificado.

## Actualizar

```bash
cd ~/iman && docker compose pull && docker compose up -d
```

## Volver atrás

```bash
IMAN_TAG=<sha-del-commit-bueno> docker compose up -d
```

## Comprobaciones

```bash
docker compose ps
docker compose logs --tail 50
docker exec cloud-caddy-1 wget -qO- http://iman:8080/vivo    # -> ok
```

Y desde fuera, que el acceso está puesto de verdad:

```bash
curl -si https://iman.ojoalprecio.com/ | grep -i '^location'   # -> /entrar
```

## DonTorrent por la Pi5

Cloudflare veta en DonTorrent las IPs de Hetzner (`error code: 1005`). Para
DonTorrent, Imán sale por un tinyproxy en la Pi5 de casa, a través de Tailscale:

- En la Pi5 (`pi@100.99.209.115`), `~/proxy-iman`: tinyproxy en Docker,
  publicado solo en la IP de Tailscale (`100.99.209.115:8888`), que acepta solo
  al servidor (`Allow 100.96.69.24`) y solo hacia el puerto 443.
- En el `.env` del servidor: `IMAN_PROXY_DONTORRENT=http://100.99.209.115:8888`.

Si la Pi5 se apaga, DonTorrent sale caído en `/salud` y el resto sigue igual.
Para comprobar el proxy desde el servidor:

```bash
curl -s -o /dev/null -w "%{http_code}\n" -x http://100.99.209.115:8888 https://dontorrent.moi/   # -> 200
```
