# 🚀 Despliegue con Coolify en Oracle Free Tier

## ¿Qué es Coolify?

Coolify es una plataforma de despliegue self-hosted (como Vercel/Netlify pero en tu servidor).
- ✅ 100% gratis y open source
- ✅ UI web intuitiva
- ✅ Autodeploy desde GitHub
- ✅ SSL automático con Let's Encrypt
- ✅ Logs en tiempo real
- ✅ Webhooks para deploy instantáneo

## Instalación en Oracle Free Tier

### 1. Conecta a tu instancia Oracle
```bash
ssh ubuntu@tu-ip-oracle
```

### 2. Instala Coolify (un comando)
```bash
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash
```

Espera 2-3 minutos. Al terminar te dará una URL:
```
✅ Coolify installed successfully!
🌐 Access it at: http://tu-ip:8000
```

### 3. Configuración inicial
1. Abre `http://tu-ip:8000` en tu navegador
2. Crea tu cuenta de admin
3. Configura tu dominio (opcional)

## Desplegar Oracle Watcher con Coolify

### Opción A: Desde GitHub (Deploy automático) ⭐

1. **En Coolify → Projects → New Project**
   - Name: `Oracle Watcher`

2. **Resources → New Resource → GitHub App**
   - Conecta tu cuenta de GitHub
   - Selecciona el repo: `RadW2020/oracle-free-tier-arm-watcher`
   - Branch: `main`

3. **Configuración del servicio:**
   - Build Pack: `Dockerfile`
   - Port: `8088`
   - Dockerfile path: `/Dockerfile`

4. **Variables de entorno:**
   ```env
   PORT=8088
   API_KEY=tu-clave-secreta
   OCI_TENANCY_ID=ocid1.tenancy...
   OCI_USER_ID=ocid1.user...
   OCI_FINGERPRINT=xx:xx:xx...
   OCI_PRIVATE_KEY_PATH=/app/key.pem
   OCI_REGION=eu-madrid-1
   OCI_COMPARTMENT_ID=ocid1.compartment...
   ```

5. **Ficheros (para key.pem):**
   - Path: `/app/key.pem`
   - Content: [pega el contenido de tu key.pem]
   - Permissions: `600`

6. **Deploy Settings:**
   - ✅ Enable Auto Deploy on Push

**¡Listo!** Cada push a `main` desplegará automáticamente en segundos.

---

### Opción B: Desde Docker Registry (más simple)

1. **En Coolify → Resources → New Resource → Docker Image**

2. **Configuración:**
   - Image: `ghcr.io/radw2020/oracle-free-tier-arm-watcher:latest`
   - Port: `8088`
   - Pull Policy: `Always` ← Importante

3. **Variables de entorno:** [igual que arriba]

4. **Deploy**

Coolify verificará cada pocos minutos si hay una nueva imagen.

---

## Ventajas de Coolify vs Watchtower

| Característica | Watchtower | Coolify |
|----------------|------------|---------|
| **Deploy automático** | ✅ Cada 1h | ⚡ Instantáneo (webhook) |
| **UI Web** | ❌ | ✅ |
| **Logs en tiempo real** | ❌ | ✅ |
| **SSL automático** | ❌ | ✅ |
| **Rollback fácil** | ❌ | ✅ |
| **Variables de entorno** | .env manual | ✅ UI |
| **Multi-app** | ❌ | ✅ |
| **Consumo RAM** | ~10MB | ~200MB |

---

## SSL con Coolify (Bonus)

Si tienes un dominio:

1. **DNS:** Apunta tu dominio a la IP de Oracle
   ```
   A     watcher.tudominio.com  →  tu-ip-oracle
   ```

2. **En Coolify:**
   - Domains → Add Domain: `watcher.tudominio.com`
   - ✅ Enable SSL (Let's Encrypt)

3. **Listo!** Tu app en `https://watcher.tudominio.com` 🎉

---

## Firewall en Oracle Cloud

No olvides abrir los puertos en OCI:

1. **OCI Console → Networking → Virtual Cloud Networks**
2. **Security Lists → Default Security List**
3. **Add Ingress Rule:**
   - Port: `8088` (para el watcher)
   - Port: `8000` (para Coolify UI)
   - Source: `0.0.0.0/0` (o tu IP para mayor seguridad)

---

## Troubleshooting

### Error: "Cannot pull image"
**Solución:** La imagen debe ser pública o configurar GitHub PAT
```bash
# En tu GitHub → Settings → Developer settings → PAT
# Crear token con scope: read:packages
# En Coolify → Settings → Registry → Add GitHub
```

### Error: "Port already in use"
**Solución:** Verifica que no haya otro servicio en 8088
```bash
sudo lsof -i :8088
# Si hay algo, mátalo o cambia el puerto
```

### Coolify no arranca
**Solución:** Verifica Docker
```bash
sudo systemctl status docker
sudo systemctl start docker
```

---

## Comandos Útiles

```bash
# Ver logs de Coolify
docker logs -f coolify

# Reiniciar Coolify
docker restart coolify

# Ver servicios corriendo
docker ps

# Ver uso de recursos
docker stats
```

---

## Resumen

**Setup inicial:**
```bash
# 1. Instalar Coolify
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash

# 2. Configurar en UI (http://tu-ip:8000)
# 3. Conectar GitHub
# 4. Deploy automático activado
```

**Workflow:**
```
git push → GitHub Actions build → Webhook → Coolify redeploy (30s)
```

**Sin Coolify (Watchtower):**
```
git push → GitHub Actions build → Watchtower check cada 1h → Update
```

---

🎯 **Recomendación:** Usa Coolify si quieres despliegues instantáneos y una UI bonita. Usa Watchtower si prefieres algo simple y sin UI.
