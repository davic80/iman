# Imán es un binario estático con las plantillas y los estáticos empotrados
# dentro (//go:embed). Eso permite que la imagen final sea un distroless con un
# único fichero: sin shell, sin gestor de paquetes, sin nada que parchear.
# Medido: 3,9 MB comprimida, y arranque instantáneo.

# --- Build ------------------------------------------------------------------
FROM golang:1.25-alpine AS build
WORKDIR /src

# Las dependencias van en su propia capa. Hoy Imán no tiene ninguna, pero en
# cuanto entren las de scraping esta capa deja de reconstruirse en cada push.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# El SHA del commit viaja dentro del binario y se ve en /salud: así se sabe
# exactamente qué versión está corriendo sin entrar al servidor.
ARG VERSION=dev

# CGO_ENABLED=0 es lo que hace el binario verdaderamente estático, requisito
# para que corra en una imagen sin libc.
# -s -w quitan la tabla de símbolos y la info de depuración (unos 30% menos).
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /iman ./cmd/iman

# Los tests corren en CI, pero repetirlos aquí garantiza que ninguna imagen
# pueda construirse a partir de código que no pasa.
RUN CGO_ENABLED=0 go test ./...

# El directorio de datos se crea aquí porque el distroless final no tiene shell
# para un mkdir. Al crear el volumen por primera vez, Docker copia el contenido
# y los permisos que encuentre en la imagen: así el volumen nace siendo del
# usuario nonroot y no de root, que si no la app no podría escribir en él.
RUN mkdir -p /datos

# --- Serve ------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /iman /iman
COPY --from=build --chown=nonroot:nonroot /datos /datos

# El estado (qué dominio funciona para cada sitio) vive aquí, en un volumen.
# Perderlo no rompe nada: solo obliga a redescubrir los dominios al arrancar.
ENV IMAN_ESTADO=/datos/estado.json

# distroless:nonroot ya corre como uid 65532. Se deja explícito para que se vea.
USER nonroot:nonroot

EXPOSE 8080

# Sin shell dentro, así que el healthcheck lo hace el propio binario contra su
# ruta /vivo. Ver el modo `-sonda` en cmd/iman.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/iman", "-sonda"]

ENTRYPOINT ["/iman"]
