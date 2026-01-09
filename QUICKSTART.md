# 🚀 Oracle Free Tier Watcher - Guía Completa

## 📋 Tabla de Contenidos

1. [Inicio Rápido (5 minutos)](#inicio-rápido)
2. [Despliegue Local (Desarrollo)](#desarrollo-local)
3. [Despliegue en Producción (Coolify)](#despliegue-en-producción)
4. [Configuración y Uso](#configuración-y-uso)
5. [Troubleshooting](#troubleshooting)

---

## 🎯 Inicio Rápido

### Requisitos
- ✅ Cuenta en Oracle Cloud (Free Tier)
- ✅ Instancia Oracle con Ubuntu/Oracle Linux
- ✅ Go instalado (para desarrollo local)

### Setup en 3 pasos

#### 1️⃣ Clonar y configurar
```bash
git clone https://github.com/RadW2020/oracle-free-tier-arm-watcher.git
cd oracleFreeTierWatcher

# Copiar archivo de configuración
cp .env.example .env
```

#### 2️⃣ Generar API Key y configurar credenciales
```bash
# Generar API Key segura
echo "API_KEY=$(openssl rand -hex 32)" >> .env

# Editar .env con tus credenciales de OCI
nano .env  # o vim, code, etc.
```

**Variables requeridas en `.env`:**
```env
# Autenticación del watcher
API_KEY=tu-clave-generada

# Credenciales de Oracle Cloud
OCI_TENANCY_ID=ocid1.tenancy.oc1..xxxxx
OCI_USER_ID=ocid1.user.oc1..xxxxx
OCI_FINGERPRINT=xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx
OCI_PRIVATE_KEY_PATH=/path/to/your/oci_api_key.pem
OCI_REGION=eu-madrid-1
OCI_COMPARTMENT_ID=ocid1.compartment.oc1..xxxxx
```

**¿Dónde obtener las credenciales?**
- `OCI_TENANCY_ID`: OCI Console → Profile → Tenancy
- `OCI_USER_ID`: OCI Console → Profile → User Settings
- `OCI_FINGERPRINT`: OCI Console → Profile → API Keys
- `OCI_PRIVATE_KEY_PATH`: Ruta al archivo `.pem` descargado
- `OCI_REGION`: Tu región (ej: `eu-madrid-1`, `us-ashburn-1`)
- `OCI_COMPARTMENT_ID`: Normalmente igual al Tenancy ID

#### 3️⃣ Desplegar

**Para desarrollo local:**
```bash
go mod download
go build -o watcher .
./watcher
```

**Para producción (Oracle Free Tier):**
Salta a la sección [Despliegue en Producción](#despliegue-en-producción)

---

## 💻 Desarrollo Local

### Instalación de Go

**macOS:**
```bash
brew install go
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt update
sudo apt install golang-go
```

**Oracle Linux / RHEL:**
```bash
sudo dnf install golang
```

### Compilar y ejecutar

```bash
# Instalar dependencias
go mod download

# Compilar
go build -o watcher .

# Ejecutar
./watcher
```

### Docker local (opcional)

```bash
# Build y run
docker-compose up -d

# Ver logs
docker-compose logs -f oracle-watcher

# Detener
docker-compose down
```

### Probar endpoints

```bash
# Health check (sin autenticación)
curl http://localhost:8088/health

# Usage con API Key
export API_KEY="tu-clave-del-env"
curl -H "X-API-Key: $API_KEY" http://localhost:8088/usage | jq

# O usar el script de prueba
chmod +x test-auth.sh
./test-auth.sh
```

### Tests unitarios

```bash
# Ejecutar todos los tests
go test -v

# Con coverage
go test -cover

# Benchmark
go test -bench=.
```

---

## � Despliegue en Producción

### Estrategia: Coolify

**Coolify** es una plataforma self-hosted (como Vercel/Netlify) que ofrece:

- ⚡ **Deploy en 30 segundos** tras cada `git push`
- 🖥️ **UI web intuitiva** para gestionar tus apps
- 🔐 **SSL automático** con Let's Encrypt
- 📊 **Logs en tiempo real**
- 🔄 **Rollback fácil** a versiones anteriores
- 🎯 **100% gratis y open source**
- 🐳 **Soporta múltiples apps** en el mismo servidor

### Flujo de trabajo final

```
┌──────────────┐
│  Local Dev   │
│  (tu Mac)    │
└──────┬───────┘
       │
       │ git push
       ▼
┌──────────────┐
│ GitHub       │
│ Actions      │ ← Compila imagen ARM64
└──────┬───────┘
       │
       │ webhook (30s)
       ▼
┌──────────────┐
│  Coolify     │
│ (Oracle ARM) │ ← Deploy automático
└──────┬───────┘
       │
       ▼
┌──────────────┐
│  Running!    │
│  https://... │
└──────────────┘
```

---

## 📦 Instalación de Coolify

### 1. Conecta a tu Oracle instance

```bash
ssh ubuntu@tu-ip-oracle
```

### 2. Instala Coolify (un solo comando)

```bash
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash
```

**Tiempo de instalación:** 2-3 minutos

**Al terminar verás:**
```
✅ Coolify installed successfully!
🌐 Access it at: http://tu-ip:8000
```

### 3. Abre el firewall en Oracle Cloud

**Importante:** Debes abrir el puerto en OCI Console:

1. Ve a **OCI Console → Networking → Virtual Cloud Networks**
2. Selecciona tu VCN
3. **Security Lists → Default Security List**
4. **Add Ingress Rule:**
   - Source CIDR: `0.0.0.0/0`
   - IP Protocol: `TCP`
   - Destination Port Range: `8000` (Coolify UI)
   - (Opcional) Puerto `80` y `443` si usas dominio

5. **En tu servidor también:**
```bash
# Ubuntu/Debian
sudo ufw allow 8000/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Oracle Linux
sudo firewall-cmd --permanent --add-port=8000/tcp
sudo firewall-cmd --permanent --add-port=80/tcp
sudo firewall-cmd --permanent --add-port=443/tcp
sudo firewall-cmd --reload
```

---

## ⚙️ Configuración en Coolify

### 1. Acceso inicial

1. Abre `http://tu-ip-oracle:8000` en tu navegador
2. Crea tu cuenta de administrador
3. Verifica tu email (o sáltalo si es para tu uso personal)

### 2. Conectar GitHub

1. **Settings → GitHub App**
2. Click en **Create GitHub App**
3. Te redirigirá a GitHub para autorizar
4. Selecciona los repositorios (o todos)
5. Instala la app de GitHub

### 3. Crear proyecto y desplegar

#### Opción A: Deploy desde GitHub (Recomendado)

1. **Projects → New Project**
   - Name: `Oracle Watcher`

2. **Resources → New Resource → Application**
   - Source: **GitHub**
   - Repository: `RadW2020/oracle-free-tier-arm-watcher`
   - Branch: `main`

3. **Build Configuration:**
   - Build Pack: **Dockerfile**
   - Dockerfile Location: `/Dockerfile`
   - Port: `8088`

4. **Environment Variables:**
   
   Click en **Add Variable** para cada una:
   ```
   PORT=8088
   API_KEY=tu-clave-secreta-generada
   OCI_TENANCY_ID=ocid1.tenancy.oc1..xxxxx
   OCI_USER_ID=ocid1.user.oc1..xxxxx
   OCI_FINGERPRINT=xx:xx:xx:xx:xx...
   OCI_PRIVATE_KEY_PATH=/app/key.pem
   OCI_REGION=eu-madrid-1
   OCI_COMPARTMENT_ID=ocid1.compartment.oc1..xxxxx
   ```

5. **Ficheros (para key.pem):**
   - Click en **Files** → **Add File**
   - Path: `/app/key.pem`
   - Content: [Pega el contenido completo de tu archivo `.pem`]
   - Permissions: `600`

6. **Deploy Settings:**
   - ✅ Enable **Auto Deploy**
   - Esto creará un webhook en GitHub automáticamente

7. **Deploy!**
   - Click en **Deploy**
   - Verás los logs en tiempo real

#### Opción B: Deploy desde Docker Registry

Si prefieres usar la imagen pre-compilada:

1. **Resources → New Resource → Docker Image**
2. **Image:** `ghcr.io/radw2020/oracle-free-tier-arm-watcher:latest`
3. **Port:** `8088`
4. Configura las mismas variables de entorno
5. Deploy

---

## � Configurar SSL (Opcional pero recomendado)

Si tienes un dominio:

### 1. Configurar DNS

Apunta tu dominio a la IP de Oracle:

```
A     watcher.tudominio.com  →  tu-ip-oracle
```

### 2. En Coolify

1. Ve a tu aplicación en Coolify
2. **Domains → Add Domain**
3. Escribe: `watcher.tudominio.com`
4. ✅ Enable **SSL (Let's Encrypt)**
5. Coolify generará el certificado automáticamente

**¡Listo!** Tu app estará en `https://watcher.tudominio.com` 🎉

---

## 🎮 Configuración y Uso

### Endpoints Disponibles

| Endpoint | Descripción | Auth | Ejemplo |
|----------|-------------|------|---------|
| `GET /health` | Health check | ❌ | `curl https://watcher.tudominio.com/health` |
| `GET /limits` | Límites Free Tier | ✅ | `curl -H "X-API-Key: $KEY" .../limits` |
| `GET /usage` | Uso detallado | ✅ | `curl -H "X-API-Key: $KEY" .../usage` |
| `GET /status` | Estado rápido | ✅ | `curl -H "X-API-Key: $KEY" .../status` |

### Ejemplo de uso con autenticación

```bash
# Guardar API Key
export API_KEY="tu-clave-del-env"

# Ver uso detallado
curl -H "X-API-Key: $API_KEY" \
  https://watcher.tudominio.com/usage | jq

# Ver solo el estado
curl -H "X-API-Key: $API_KEY" \
  https://watcher.tudominio.com/status | jq '.status'
```

### Respuesta de ejemplo

```json
{
  "status": "OK",
  "maxUsagePercentage": 50,
  "warnings": [],
  "timestamp": "2026-01-10T00:00:00Z",
  "configured": true,
  "usage": {
    "compute": {
      "arm": {
        "ocpus": { "used": 2, "limit": 4, "percentage": 50 },
        "memoryGB": { "used": 12, "limit": 24, "percentage": 50 },
        "instances": 1
      }
    },
    "blockStorage": {
      "total": { "used": 100, "limit": 200, "percentage": 50 }
    }
  }
}
```

### Estados posibles

| Status | Porcentaje | Descripción |
|--------|-----------|-------------|
| `OK` | < 60% | Todo bien |
| `ATTENTION` | 60-80% | Precaución |
| `WARNING` | 80-90% | Revisar |
| `CRITICAL` | > 90% | Límite cerca |

---

## 🔧 Troubleshooting

### "Unauthorized" al acceder a endpoints

**Problema:** Respuesta 401 Unauthorized

**Solución:** Asegúrate de pasar el header `X-API-Key`
```bash
curl -H "X-API-Key: tu-clave" https://watcher.tudominio.com/usage
```

---

### "OCI credentials not configured"

**Problema:** La app responde con estado `NOT_CONFIGURED`

**Solución:** Verifica las variables de entorno en Coolify:
1. Ve a tu app → **Environment Variables**
2. Verifica que todas las variables `OCI_*` estén configuradas
3. Especialmente verifica `OCI_PRIVATE_KEY_PATH=/app/key.pem`

---

### "Private key file not found"

**Problema:** Error al iniciar: "Private key file not found"

**Solución:** 
1. En Coolify → **Files**
2. Verifica que `/app/key.pem` existe
3. Contenido debe ser tu clave privada completa (incluye headers):
   ```
   -----BEGIN PRIVATE KEY-----
   ...
   -----END PRIVATE KEY-----
   ```
4. Permissions debe ser `600`

---

### Coolify no arranca

**Solución:**
```bash
# Verificar Docker
sudo systemctl status docker
sudo systemctl start docker

# Reiniciar Coolify
docker restart coolify

# Ver logs
docker logs -f coolify
```

---

### Deploy falla en GitHub Actions

**Solución:**
1. Ve a GitHub → Actions → Ver el error
2. Usualmente es problema de permisos
3. En GitHub → Settings → Actions → Workflow permissions
4. Selecciona "Read and write permissions"

---

### No puedo acceder a Coolify UI (puerto 8000)

**Solución:**
1. Verifica firewall en Oracle Cloud (Security Lists)
2. Verifica firewall local:
   ```bash
   sudo ufw status
   sudo ufw allow 8000/tcp
   ```
3. Verifica que Coolify esté corriendo:
   ```bash
   docker ps | grep coolify
   ```

---

## ✅ Checklist de Producción

- [ ] **Oracle Free Tier configurado**
  - [ ] Instancia ARM (VM.Standard.A1.Flex)
  - [ ] 4 OCPUs, 24GB RAM
  - [ ] Región correcta (Home Region)
  
- [ ] **Credenciales configuradas**
  - [ ] `.env` con todas las variables OCI
  - [ ] `API_KEY` generada con `openssl rand -hex 32`
  - [ ] Archivo `.pem` con permisos 600
  
- [ ] **Coolify instalado**
  - [ ] Instalación completada
  - [ ] Acceso a UI funcionando
  - [ ] GitHub conectado
  
- [ ] **App desplegada**
  - [ ] Proyecto creado en Coolify
  - [ ] Variables de entorno configuradas
  - [ ] Archivo `key.pem` montado correctamente
  - [ ] Webhook de GitHub configurado
  - [ ] Deploy exitoso
  
- [ ] **Verificación**
  - [ ] `/health` responde OK
  - [ ] Logs muestran "OCI credentials validated successfully"
  - [ ] `/usage` devuelve datos correctos
  - [ ] Auto-deploy funciona (prueba con un push)
  
- [ ] **Seguridad**
  - [ ] API_KEY configurada y funcionando
  - [ ] Firewall configurado en OCI
  - [ ] (Opcional) SSL configurado con dominio
  - [ ] Alertas de presupuesto en OCI Console

---

## 📊 Mejoras Incluidas

Este proyecto incluye:

- ✅ **Autenticación con API Key** - Protege tus endpoints
- ✅ **Logging estructurado** - Logs en JSON con zerolog
- ✅ **Llamadas paralelas a OCI** - 3-5x más rápido con goroutines
- ✅ **Validación de credenciales** - Verifica al iniciar
- ✅ **Tests unitarios** - Cobertura básica
- ✅ **GitHub Actions** - Build automático ARM64
- ✅ **Deploy automático** - Con Coolify
- ✅ **Puerto normalizado** - 8088 en todo el proyecto

---

## 🆘 Recursos Adicionales

- **Documentación:**
  - [README.md](README.md) - Información general del proyecto
  - [SECURITY.md](SECURITY.md) - Guía de seguridad
  - [CHANGELOG.md](CHANGELOG.md) - Historial de cambios

- **Enlaces externos:**
  - [Coolify Documentation](https://coolify.io/docs)
  - [Oracle Free Tier](https://www.oracle.com/cloud/free/)
  - [OCI Go SDK](https://github.com/oracle/oci-go-sdk)

- **Ayuda:**
  - Issues: https://github.com/RadW2020/oracle-free-tier-arm-watcher/issues
  - Coolify Discord: https://discord.gg/coolify

---

## 🎉 ¡Listo!

Tu Oracle Free Tier Watcher está configurado y desplegado.

**Workflow final:**
```
git push → GitHub Actions (5 min) → Webhook → Coolify (30s) → ✅ Live!
```

**Disfruta monitoreando tu Oracle Free Tier sin sorpresas en la factura!** 🚀
