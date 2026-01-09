# 🚀 Quick Start - Oracle Free Tier Watcher

## Inicio Rápido (5 minutos)

### 1️⃣ Clonar y configurar
```bash
git clone https://github.com/RadW2020/oracle-free-tier-arm-watcher.git
cd oracleFreeTierWatcher
```

### 2️⃣ Configurar credenciales
```bash
# Copiar el archivo de ejemplo
cp .env.example .env

# Generar API Key segura
echo "API_KEY=$(openssl rand -hex 32)" >> .env

# Editar .env con tus credenciales de OCI
nano .env  # o vim, code, etc.
```

**Necesitas configurar:**
- `OCI_TENANCY_ID` - De OCI Console → Profile → Tenancy
- `OCI_USER_ID` - De OCI Console → Profile → User Settings
- `OCI_FINGERPRINT` - De OCI Console → Profile → API Keys
- `OCI_PRIVATE_KEY_PATH` - Ruta a tu archivo `.pem`
- `OCI_REGION` - Región de tu instancia (ej: `eu-madrid-1`)
- `OCI_COMPARTMENT_ID` - ID del compartimento (normalmente = tenancy)

### 3️⃣ Ejecutar

**Desarrollo local:**
```bash
# Instalar Go si no lo tienes
brew install go  # macOS
# o sudo apt install golang-go  # Linux

# Compilar y ejecutar
go mod download
go build -o watcher .
./watcher
```

**Producción en Oracle Free Tier:**
```bash
# Instalar Coolify (una sola vez)
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash

# Luego configura el deploy desde la UI
# http://tu-ip-oracle:8000
```

👉 **[Ver guía completa de Coolify](DEPLOY_COOLIFY.md)**

### 4️⃣ Probar
```bash
# Health check (sin autenticación)
curl http://localhost:8088/health

# Usage (con autenticación)
export API_KEY="tu-clave-del-env"
curl -H "X-API-Key: $API_KEY" http://localhost:8088/usage

# O usar el script de prueba
./test-auth.sh
```

---

## 📚 Endpoints Disponibles

### GET /health
Health check simple (sin autenticación)
```bash
curl http://localhost:8088/health
```

### GET /limits
Límites de la Free Tier
```bash
curl -H "X-API-Key: $API_KEY" http://localhost:8088/limits
```

### GET /usage
Uso detallado de todos los recursos
```bash
curl -H "X-API-Key: $API_KEY" http://localhost:8088/usage | jq
```

### GET /status
Estado rápido (OK/ATTENTION/WARNING/CRITICAL)
```bash
curl -H "X-API-Key: $API_KEY" http://localhost:8088/status
```

---

## 🚀 Despliegue en Producción

Para despliegue en tu Oracle Free Tier, usa **Coolify**:

```bash
# En tu instancia Oracle
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash
```

Accede a `http://tu-ip:8000` y configura tu app.

👉 **[Guía completa de Coolify](DEPLOY_COOLIFY.md)**

**Beneficios:**
- ⚡ Deploy en 30 segundos
- 🖥️ UI web intuitiva
- 🔐 SSL automático
- 📊 Logs en tiempo real

---

## 🔧 Troubleshooting

### "Unauthorized" al acceder a endpoints
**Solución:** Asegúrate de pasar el header `X-API-Key`
```bash
curl -H "X-API-Key: tu-clave" http://localhost:8088/usage
```

### "OCI credentials not configured"
**Solución:** Verifica que tu `.env` tenga todas las variables
```bash
cat .env | grep OCI_
```

### "Private key file not found"
**Solución:** Verifica la ruta en `OCI_PRIVATE_KEY_PATH`
```bash
ls -l $(grep OCI_PRIVATE_KEY_PATH .env | cut -d= -f2)
```

### Error de permisos en Docker
**Solución:** El archivo `.pem` debe tener permisos 600
```bash
chmod 600 /path/to/your/key.pem
```

---

## 🔐 Seguridad

### Generar API Key
```bash
openssl rand -hex 32
```

### Rotar credenciales
1. Ve a OCI Console → Profile → API Keys
2. Elimina la clave antigua
3. Genera una nueva
4. Actualiza `.env`
5. Reinicia el servicio

### Modo desarrollo (logs legibles)
```bash
ENV=development ./watcher
```

---

## 📖 Documentación Completa

- [README.md](README.md) - Documentación principal
- [SECURITY.md](SECURITY.md) - Guía de seguridad
- [CHANGELOG.md](CHANGELOG.md) - Resumen de mejoras
- [test-auth.sh](test-auth.sh) - Script de pruebas

---

## 🆘 Ayuda

**Issues:** https://github.com/RadW2020/oracle-free-tier-arm-watcher/issues

**Logs útiles:**
```bash
# Ver logs del watcher
docker-compose logs oracle-watcher

# Ver logs de Watchtower
docker-compose logs watchtower

# Ver todos los logs
docker-compose logs -f
```

---

## ✅ Checklist de Producción

- [ ] `.env` configurado con credenciales reales
- [ ] `API_KEY` generada con `openssl rand -hex 32`
- [ ] Archivo `.pem` con permisos 600
- [ ] **Coolify instalado** en Oracle instance
- [ ] **App desplegada** en Coolify
- [ ] **Webhook configurado** para auto-deploy
- [ ] Endpoints responden correctamente
- [ ] Logs muestran "OCI credentials validated successfully"
- [ ] Configurar alertas de presupuesto en OCI Console
- [ ] **(Opcional)** Configurar dominio y SSL en Coolify

---

**¡Listo!** 🎉 Tu Oracle Free Tier Watcher está corriendo.
