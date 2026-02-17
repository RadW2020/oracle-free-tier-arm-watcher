# Etapa 1: Compilación
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Instalar git si es necesario para algunas dependencias
RUN apk add --no-cache git

# Copiar archivos de dependencias
COPY go.mod go.sum ./
RUN go mod download

# Copiar el resto del código
COPY . .

# Compilar de forma estática
RUN CGO_ENABLED=0 GOOS=linux go build -o watcher .

# Etapa 2: Imagen mínima de ejecución
FROM alpine:latest

# IMPORTANTE: Necesitamos los certificados para conectar con la API de Oracle (HTTPS)
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copiar el binario desde el builder
COPY --from=builder /app/watcher .

# Puerto por defecto
EXPOSE 8088

# Ejecutar
CMD ["./watcher"]
