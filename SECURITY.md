# 🔒 Guía de Seguridad - Oracle Free Tier Watcher

## Archivos Sensibles

### ✅ Estado Actual: SEGURO
Los siguientes archivos **NO están trackeados en Git**:
- ✅ `.env` - Credenciales de OCI y API Key
- ✅ `key.pem` - Clave privada de OCI
- ✅ `*.pem` - Cualquier otro archivo de clave

El `.gitignore` está correctamente configurado para protegerlos.

## 🔐 Configuración de Seguridad

### 1. Generar API Key segura

```bash
# Generar una clave aleatoria de 64 caracteres hexadecimales
openssl rand -hex 32
```

Añade esta clave a tu `.env`:
```env
API_KEY=abc123...  # Tu clave generada
```

### 2. Proteger endpoints en producción

Si `API_KEY` está configurada:
- ✅ `/health` - **Público** (para health checks de Docker/Kubernetes)
- 🔒 `/usage` - **Protegido** (requiere `X-API-Key`)
- 🔒 `/status` - **Protegido** (requiere `X-API-Key`)
- 🔒 `/limits` - **Protegido** (requiere `X-API-Key`)

### 3. Ejemplo de uso con autenticación

```bash
# Guardar tu API Key en variable de entorno
export API_KEY="tu-clave-secreta-aquí"

# Llamar al endpoint
curl -H "X-API-Key: $API_KEY" http://localhost:8088/usage
```

### 4. Logs de seguridad

El sistema registra todos los intentos de acceso:

```json
{
  "level": "warn",
  "ip": "192.168.1.1",
  "path": "/usage",
  "time": 1704834567,
  "message": "Unauthorized request - invalid API key"
}
```

## ⚠️ Verificación de Seguridad

Antes de hacer commit/push, verifica:

```bash
# Ver archivos trackeados por Git
git ls-files

# Buscar archivos sensibles (no debería devolver nada)
git ls-files | grep -E '\.pem$|^\.env$'

# Ver status actual
git status
```

Si encuentras archivos sensibles trackeados:

```bash
# Remover del index pero mantener el archivo local
git rm --cached .env
git rm --cached key.pem

# Commit del cambio
git commit -m "Remove sensitive files from Git"
```

## 🔑 Rotación de Credenciales

Si comprometes accidentalmente tus credenciales:

1. **Rotar API Key de OCI:**
   - Ve a OCI Console → Profile → API Keys
   - Elimina la clave comprometida
   - Genera una nueva
   - Actualiza tu `.env`

2. **Rotar API_KEY del watcher:**
   ```bash
   # Generar nueva clave
   openssl rand -hex 32
   # Actualizar en .env
   ```

3. **Rotar en Docker/Coolify:**
   - Actualiza las variables de entorno
   - Reinicia el contenedor

## 📨 Seguridad en Postiz (Docker Compose)

Al desplegar Postiz, ten en cuenta estas medidas adicionales:

1. **JWT Secret:** Genera un secreto único y largo para `JWT_SECRET`.
2. **Registro Cerrado:** Por defecto, hemos configurado `DISABLE_REGISTRATION=true`. El primer usuario debe registrarse con esta variable en `false` y luego volver a ponerla en `true`.
3. **Temporal UI:** La interfaz de Temporal activa no tiene autenticación por defecto. Se ha movido al puerto 8085, pero es recomendable:
   - No exponerla públicamente si no es necesario.
   - Usar un Basic Auth vía Traefik si necesitas acceso externo.
4. **Base de Datos:** Los contenedores de Postgres y Redis no exponen puertos al host (0.0.0.0) para evitar ataques externos. Solo se comunican por la red interna de Docker.

---

## 🛡️ Mejores Prácticas

1. **Nunca commitear `.env`** - Aunque está en `.gitignore`, verifica siempre
2. **Rotar claves periódicamente** - Cada 90 días mínimo
3. **Usar claves diferentes por entorno** - Dev, Staging, Production
4. **Monitorear logs** - Revisar intentos de acceso no autorizados
5. **HTTPS en producción** - Usar reverse proxy (nginx/Traefik) con SSL

## 📋 Checklist de Despliegue

- [ ] `.env` configurado con credenciales únicas
- [ ] `API_KEY` generada con `openssl rand -hex 32`
- [ ] Archivo `key.pem` montado correctamente en Docker
- [ ] Permisos del archivo `key.pem` son 600 (`chmod 600 key.pem`)
- [ ] Reverse proxy configurado con HTTPS
- [ ] Firewall configurado (solo puertos necesarios)
- [ ] Logs monitoreados
