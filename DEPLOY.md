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
  cloud-caddy-1  ← termina TLS (Let's Encrypt), pide contraseña y enruta
        │  red docker: cloud_default
        ▼
      imán       ← :8080, solo alcanzable desde dentro
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
```

Autentica contra GHCR una sola vez y arranca:

```bash
docker compose up -d
```

## Contraseña

La instancia es privada. La autenticación la pone Caddy, no la app. Genera el
hash en el propio contenedor de Caddy:

```bash
docker exec -it cloud-caddy-1 caddy hash-password --plaintext 'tu-contraseña'
```

Y añade el hostname al Caddyfile compartido (`~/padelscores/cloud/Caddyfile`):

```
iman.ojoalprecio.com {
    basic_auth {
        david $2a$14$...el-hash-que-te-dio...
    }
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

Y desde fuera, que la contraseña está puesta de verdad:

```bash
curl -si https://iman.ojoalprecio.com/ | head -1     # -> 401
curl -su david:'tu-contraseña' https://iman.ojoalprecio.com/salud | head -20
```
