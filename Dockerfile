# Etapa 1: Compilación
# Go 1.25+ porque lo exige el SDK de MCP (github.com/modelcontextprotocol/go-sdk).
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Instalar git si es necesario para algunas dependencias
RUN apk add --no-cache git

# Copiar archivos de dependencias
COPY go.mod go.sum ./
RUN go mod download

# Copiar el resto del código
COPY . .

# Compilar de forma estática. Los escenarios de fixtures van embebidos en el
# binario (internal/source/scenarios), así que el demo no necesita ficheros.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.version=${VERSION}" -o watcher .

# Etapa 2: Imagen mínima de ejecución
FROM alpine:latest

# IMPORTANTE: Necesitamos los certificados para conectar con la API de Oracle (HTTPS)
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copiar el binario desde el builder
COPY --from=builder /app/watcher .

# Puerto por defecto
EXPOSE 8088

HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8088/health >/dev/null || exit 1

# Ejecutar
CMD ["./watcher"]
