# 🚀 Deploy Directo con GitHub Actions (Alternativa simple)

Si no quieres instalar Coolify, puedes hacer que GitHub Actions despliegue directamente vía SSH.

## Configuración

### 1. Generar SSH Key para GitHub Actions

En tu Oracle instance:
```bash
ssh-keygen -t ed25519 -C "github-actions" -f ~/.ssh/github_actions
cat ~/.ssh/github_actions.pub >> ~/.ssh/authorized_keys
cat ~/.ssh/github_actions  # Copia esto
```

### 2. Configurar Secrets en GitHub

1. Ve a: `https://github.com/RadW2020/oracle-free-tier-arm-watcher/settings/secrets/actions`
2. Añade estos secrets:
   - `SSH_PRIVATE_KEY`: La clave privada que copiaste
   - `SSH_HOST`: Tu IP de Oracle
   - `SSH_USER`: `ubuntu` (o el usuario que uses)

### 3. Crear workflow de deploy

Crea `.github/workflows/deploy-ssh.yml`:

```yaml
name: Deploy to Oracle

on:
  push:
    branches: [ "main" ]
  workflow_dispatch:

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Build and push Docker image
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          platforms: linux/arm64
          tags: ghcr.io/radw2020/oracle-free-tier-arm-watcher:latest

      - name: Deploy to Oracle
        uses: appleboy/ssh-action@master
        with:
          host: ${{ secrets.SSH_HOST }}
          username: ${{ secrets.SSH_USER }}
          key: ${{ secrets.SSH_PRIVATE_KEY }}
          script: |
            cd ~/oracle-watcher
            docker-compose pull
            docker-compose up -d
            docker image prune -f
```

## Ventajas

- ✅ Deploy inmediato (no espera 1 hora)
- ✅ No requiere Coolify/Watchtower
- ✅ Control total del proceso
- ✅ Logs en GitHub Actions

## Desventajas

- ❌ Expones SSH al mundo (mitigar con fail2ban)
- ❌ Más manual que Coolify
- ❌ Sin UI web

## Comparación

| Método | Deploy | Complejidad | RAM | UI |
|--------|--------|-------------|-----|-----|
| **Watchtower** | 1 hora | Baja | 10MB | ❌ |
| **Coolify** | Instantáneo | Media | 200MB | ✅ |
| **GitHub Actions SSH** | Instantáneo | Baja | 0MB | ❌ |

## Recomendación

- 🥇 **Coolify** - Si quieres la mejor experiencia
- 🥈 **Watchtower** - Si quieres simplicidad (ya lo tienes)
- 🥉 **GitHub Actions SSH** - Si quieres velocidad sin overhead
