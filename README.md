# Oracle Free Tier Watcher 🔍

Servicio en Go para monitorear el uso de Oracle Cloud Free Tier y evitar cargos inesperados.

## ¿Por qué Go?

- **Binario único** - No necesitas instalar Node.js ni dependencias en el servidor
- **Muy eficiente** - Usa poca RAM y CPU (ideal para Free Tier)
- **Fácil despliegue** - Solo copias un archivo y listo

## Endpoints

| Endpoint | Descripción | Auth |
|----------|-------------|------|
| `GET /usage` | Uso detallado de todos los recursos con porcentajes | ✅ |
| `GET /status` | Estado rápido (OK/ATTENTION/WARNING/CRITICAL) | ✅ |
| `GET /health` | Health check simple | ❌ |
| `GET /limits` | Límites de la Free Tier | ✅ |

> **🔒 Autenticación:** Los endpoints protegidos requieren el header `X-API-Key` con tu clave configurada en el `.env`.

## Instalación de Go

### macOS
```bash
brew install go
```

### Linux (Ubuntu/Debian)
```bash
sudo apt update
sudo apt install golang-go
```

### Oracle Linux / RHEL
```bash
sudo dnf install golang
```

## Setup del proyecto

```bash
# Clonar el repo
git clone https://github.com/RadW2020/oracle-free-tier-arm-watcher.git
cd oracleFreeTierWatcher

# Descargar dependencias
go mod tidy

# Compilar
go build -o watcher .

# Ejecutar
./watcher
```

## Configuración

1. Crea el archivo `.env` basándote en `.env.example`:

```bash
cp .env.example .env
```

2. Configura tus credenciales de OCI:
   - Ve a **OCI Console → Profile → API Keys**
   - Genera una nueva API Key y descarga el archivo `.pem`
   - Copia los valores a tu `.env`

```env
PORT=8088

# API Key para proteger los endpoints (recomendado)
API_KEY=$(openssl rand -hex 32)

OCI_TENANCY_ID=ocid1.tenancy.oc1..xxxxx
OCI_USER_ID=ocid1.user.oc1..xxxxx
OCI_FINGERPRINT=xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx:xx
OCI_PRIVATE_KEY_PATH=/path/to/your/oci_api_key.pem
OCI_REGION=eu-madrid-1
OCI_COMPARTMENT_ID=ocid1.compartment.oc1..xxxxx
```

### 🔒 Seguridad

Si configuras `API_KEY`, **todos los endpoints (excepto `/health`) requerirán autenticación**:

```bash
# Sin API Key (público)
curl http://localhost:8088/usage

# Con API Key
curl -H "X-API-Key: tu-clave-secreta" http://localhost:8088/usage
```

## Ejemplo de respuesta `/usage`

```json
{
  "status": "OK",
  "maxUsagePercentage": 50,
  "warnings": [],
  "timestamp": "2024-12-29T16:30:00Z",
  "configured": true,
  "usage": {
    "compute": {
      "arm": {
        "ocpus": { "used": 2, "limit": 4, "percentage": 50 },
        "memoryGB": { "used": 12, "limit": 24, "percentage": 50 },
        "instances": 1
      },
      "amd": {
        "instances": { "used": 0, "limit": 2, "percentage": 0 }
      }
    },
    "blockStorage": {
      "total": { "used": 100, "limit": 200, "percentage": 50 }
    },
    "objectStorage": {
      "total": { "used": 2.5, "limit": 10, "percentage": 25 }
    },
    "loadBalancer": {
      "count": { "used": 0, "limit": 1, "percentage": 0 }
    }
  }
}
```

## Estados posibles

| Status | Significado |
|--------|-------------|
| `OK` | Uso < 60% |
| `ATTENTION` | Uso entre 60-80% |
| `WARNING` | Uso entre 80-90% |
| `CRITICAL` | Uso > 90% |

## Free Tier Limits (Always Free)

- **Compute ARM (Ampere A1)**: 4 OCPUs, 24GB RAM
- **Compute AMD**: 2 instancias micro
- **Block Storage**: 200GB total
- **Object Storage**: 10GB
- **Load Balancer**: 1 instancia (10 Mbps)
- **Bandwidth**: 10TB/mes egress

## Despliegue en Oracle Cloud

```bash
# En tu máquina local, compilar para Linux:
GOOS=linux GOARCH=arm64 go build -o watcher .

# Copiar al servidor:
scp watcher ubuntu@tu-servidor:/home/ubuntu/

# En el servidor:
chmod +x watcher
./watcher
```

### Ejecutar como servicio (systemd)

Crear `/etc/systemd/system/oracle-watcher.service`:
```ini
[Unit]
Description=Oracle Free Tier Watcher
After=network.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu
ExecStart=/home/ubuntu/watcher
Restart=always

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable oracle-watcher
sudo systemctl start oracle-watcher
```

### 🔄 Despliegue Automático

Tienes varias opciones para que tu app se actualice automáticamente:

#### **Opción 1: Watchtower** (Ya incluido en `docker-compose.yml`)
- ⏰ Actualiza cada hora automáticamente
- 📦 10MB RAM
- ✅ Cero configuración

```bash
docker-compose up -d  # Watchtower ya está incluido
```

#### **Opción 2: Coolify** (Recomendado - Deploy instantáneo)
- ⚡ Deploy en 30 segundos tras `git push`
- 🖥️ UI web bonita
- 🔐 SSL automático
- 📊 Logs en tiempo real

👉 **[Ver guía completa de Coolify](DEPLOY_COOLIFY.md)**

#### **Opción 3: GitHub Actions + SSH** (Simple y rápido)
- ⚡ Deploy inmediato
- 🎯 Sin dependencias
- 📝 Control total

👉 **[Ver guía de GitHub Actions](DEPLOY_GITHUB_ACTIONS.md)**

**Comparación:**

| Método | Velocidad | RAM | Complejidad | Recomendado para |
|--------|-----------|-----|-------------|------------------|
| Watchtower | 1 hora | 10MB | ⭐ | Comenzar rápido |
| Coolify | 30 seg | 200MB | ⭐⭐ | Mejor experiencia |
| GitHub Actions | 30 seg | 0MB | ⭐⭐ | Minimalistas |


## Aprendiendo Go

### Conceptos clave en este proyecto:

1. **Packages** - Todo código Go pertenece a un paquete
2. **Structs** - Como clases pero sin herencia
3. **Interfaces** - Definen comportamiento (implícitas)
4. **Error handling** - Errores como valores, no excepciones
5. **HTTP Server** - Librería estándar muy potente
6. **JSON tags** - Controlan serialización
7. **Goroutines** - Concurrencia nativa (llamadas paralelas a OCI)
8. **Channels** - Comunicación entre goroutines
9. **Middleware** - Patrón para autenticación HTTP

## 🚀 Configuración de la Instancia (¡IMPORTANTE!)

Para aprovechar al máximo la Free Tier y que este monitor tenga sentido, asegúrate de configurar tu instancia en Oracle Cloud de la siguiente manera:

- **Imagen:** Oracle Linux o Ubuntu (ambas funcionan bien con Go/Docker/Coolify).
- **Shape:** Debes seleccionar **`VM.Standard.A1.Flex`** (procesador Ampere ARM).
- **Recursos:** Configúralo con **4 OCPUs** y **24 GB de RAM**. Esta es la configuración máxima gratuita.
- **Región:** Asegúrate de crearla en tu **Home Region** (la que elegiste al registrarte), de lo contrario te cobrarán.

> **Nota:** Si eliges las instancias AMD (Micro), solo tendrás 1GB de RAM y 0.25 OCPU, lo cual es insuficiente para correr Coolify cómodamente.

## 🛡️ Red de Seguridad (Configuración en OCI)

Aunque este monitor es fiable, la red de seguridad definitiva es configurar una **Alerta de Presupuesto** en la consola de Oracle:

1. Ve a **Billing & Cost Management → Budgets**.
2. Crea un presupuesto (Create Budget).
3. Ponle un nombre (ej. "Seguridad Free Tier").
4. **Target Amount:** 1.00 (el mínimo).
5. Configura una regla de alerta (Threshold Rule):
   - **Threshold:** 0.01 (1% del presupuesto).
   - **Type:** Actual (o Forecasted para que te avise antes).
   - **Email:** Tu dirección.
   
*Si por algún error cualquier cosa te gasta 0,01€, Oracle te enviará un email inmediatamente.*

## ✅ Mejoras Implementadas

- [x] **🔒 Autenticación con API Key:** Protege los endpoints con `X-API-Key` header
- [x] **📊 Logging estructurado:** Logs en JSON con zerolog para mejor observabilidad  
- [x] **⚡ Llamadas paralelas a OCI:** Uso de goroutines para reducir tiempo de respuesta
- [x] **✔️ Validación de credenciales:** Verifica que `.env` esté correctamente configurado al iniciar
- [x] **📝 Puerto normalizado:** Puerto 8088 por defecto consistente en todo el proyecto
- [x] **✅ Monitoreo de IPs públicas:** Ya incluido en los endpoints

## 📋 Próximos Pasos / TODO

- [ ] **Configuración Instancia:** Asegurarse de elegir el Shape **`VM.Standard.A1.Flex`** (ARM Ampere) con 4 OCPUs y 24GB RAM.
- [ ] **Despliegue:** Configurar en **Coolify** apuntando al repositorio de GitHub para actualizaciones automáticas.
- [ ] Instalar Go (`brew install go`) y compilar localmente para probar.
- [ ] Configurar `.env` con las credenciales reales de OCI.
- [ ] Mapear el volumen de la clave `.pem` correctamente en `docker-compose.yml`.
- [ ] **Añadir alertas automáticas:** Integrar notificaciones (Discord/Telegram o Email vía SMTP) si el uso pasa del 80%.
- [ ] **Gráfico de uso:** Endpoint opcional para generar una pequeña tabla o gráfico en ASCII/HTML.
- [ ] **Health Check de instancia:** Si el script detecta uso de CPU < 15%, avisar que la instancia corre riesgo de ser borrada por Oracle.
- [ ] **Tests unitarios:** Añadir tests para las funciones de cálculo de porcentajes
- [ ] **Métricas Prometheus:** Exponer métricas para integración con Grafana
