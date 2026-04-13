# 🚀 Quick Start - Oracle Free Tier Watcher

## Inicio Rápido (5 minutos)

### 1. Clonar y configurar

```bash
git clone https://github.com/RadW2020/oracle-free-tier-arm-watcher.git
cd oracleFreeTierWatcher
cp .env.example .env
```

### 2. Configurar `.env`

```bash
# Generar API Key
echo "API_KEY=$(openssl rand -hex 32)" >> .env

# Editar con tus credenciales de OCI
nano .env
```

**Credenciales necesarias:**

- `OCI_TENANCY_ID`, `OCI_USER_ID`, `OCI_FINGERPRINT` → OCI Console → Profile
- `OCI_PRIVATE_KEY_PATH` → Ruta a tu archivo `.pem`
- `OCI_REGION` → Tu región (ej: `eu-madrid-1`)
- `OCI_COMPARTMENT_ID` → Normalmente igual al Tenancy ID

### 3. Test local (opcional)

```bash
go mod download
go build -o watcher .
./watcher
```

---

## Despliegue en Producción (Coolify)

### Instalar Coolify en Oracle Free Tier

```bash
# SSH a tu instancia
ssh ubuntu@tu-ip-oracle

# Instalar Coolify (2-3 min)
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | sudo bash
```

### Abrir firewall en OCI

1. **OCI Console → Networking → VCN → Security Lists**
2. **Add Ingress Rule:**
   - Puerto: `8000` (Coolify UI)
   - Source: `0.0.0.0/0`

### Configurar en Coolify

1. Abre `http://tu-ip:8000`
2. Crea cuenta admin
3. **Settings → GitHub App** → Conecta tu cuenta
4. **Projects → New → Application**
   - Source: GitHub
   - Repo: `RadW2020/oracle-free-tier-arm-watcher`
   - Branch: `main`
5. **Build:**
   - Build Pack: `Dockerfile`
   - Port: `8088`
6. **Environment Variables:**
   ```
   PORT=8088
   API_KEY=tu-clave
   OCI_TENANCY_ID=...
   OCI_USER_ID=...
   OCI_FINGERPRINT=...
   OCI_PRIVATE_KEY_PATH=/app/key.pem
   OCI_REGION=...
   OCI_COMPARTMENT_ID=...
   ```
7. **Files → Add File:**
   - Path: `/app/key.pem`
   - Content: [pega tu archivo .pem]
   - Permissions: `600`
8. **Enable Auto Deploy** ✅
9. **Deploy!**

---

## Uso

### Endpoints

```bash
# Health (sin auth)
curl https://tu-app.com/health

# Usage (con auth)
curl -H "X-API-Key: tu-clave" https://tu-app.com/usage | jq
```

### Estados

| Status    | %      | Acción          |
| --------- | ------ | --------------- |
| OK        | <60%   | ✅ Todo bien    |
| ATTENTION | 60-80% | ⚠️ Revisar      |
| WARNING   | 80-90% | 🟡 Precaución   |
| CRITICAL  | >90%   | 🔴 Límite cerca |

---

## SSL (Opcional)

Si tienes dominio:

1. DNS: `A watcher.tudominio.com → tu-ip`
2. Coolify → Domains → Add Domain
3. Enable SSL ✅

---

## Troubleshooting

### Error: "Unauthorized"

→ Asegúrate de pasar `X-API-Key` en el header

### Error: "OCI not configured"

→ Verifica variables de entorno en Coolify

### Error: "Private key not found"

→ Verifica que `/app/key.pem` existe en Files

### Coolify no accesible

```bash
# Verificar firewall
sudo ufw allow 8000/tcp

# Verificar Docker
sudo systemctl status docker
docker ps | grep coolify
```

---

## ✅ Checklist

- [ ] Instancia Oracle ARM (4 OCPUs, 24GB)
- [ ] Coolify instalado
- [ ] Firewall abierto (puerto 8000)
- [ ] GitHub conectado
- [ ] Variables de entorno configuradas
- [ ] Archivo `key.pem` añadido
- [ ] Auto-deploy activado
- [ ] `/health` responde OK

---

---

**Listo! Tu infraestructura Oracle Free Tier está ahora bajo control y a pleno rendimiento.** 🎉
