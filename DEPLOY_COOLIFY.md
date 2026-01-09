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

## ✨ Ventajas de Coolify

- ⚡ **Deploy instantáneo** - 30 segundos después de `git push`
- 🖥️ **UI web intuitiva** - Gestiona todo visualmente
- 🔐 **SSL automático** - Let's Encrypt integrado
- 📊 **Logs en tiempo real** - Debug fácil
- 🔄 **Rollback sencillo** - Vuelve a cualquier versión
- 🎯 **Webhooks** - Integración con GitHub
- 🐳 **Multi-stack** - Soporta Docker, Dockerfile, y más

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

## 🎉 ¡Listo!

Ahora cada vez que hagas `git push`, tu app se desplegará automáticamente en Oracle Free Tier en ~30 segundos.

**Workflow:**
```
Local → git push → GitHub Actions → Webhook → Coolify → Deploy ✅
```
