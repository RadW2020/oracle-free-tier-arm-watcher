# 📨 Postiz Deployment Context

Este proyecto ha sido desplegado en **Coolify** sobre la infraestructura de **Oracle Cloud (OCI)** usando la arquitectura completa recomendada para febrero de 2026.

## 🚀 Detalles del Despliegue Final

- **Instancia OCI:** `TU_IP_PUBLICA` (Ubuntu ARM)
- **URL Postiz:** [https://postiz.tu-dominio.com](https://postiz.tu-dominio.com)
- **Temporal UI:** [http://TU_IP_PUBLICA:8085](http://TU_IP_PUBLICA:8085)
- **Panel Coolify:** [http://TU_IP_PUBLICA:8000](http://TU_IP_PUBLICA:8000)

## 🏗️ Arquitectura "Ultimate Golden Compose"

El despliegue se realizó mediante un **Docker Compose personalizado** inyectado directamente en la base de datos de Coolify para evitar que se sobrescribiera con las configuraciones por defecto del repo oficial.

### Componentes:

1.  **postiz**: La aplicación principal (Frontend + Backend + Orchestrator).
2.  **postiz-postgres**: Base de datos dedicada para Postiz.
3.  **postiz-redis**: Caché y colas para Postiz.
4.  **temporal**: El motor de flujos de trabajo (Workflow Engine).
5.  **temporal-postgresql**: Base de datos para la persistencia de Temporal.
6.  **temporal-elasticsearch**: Motor de búsqueda para la visibilidad de Temporal.
7.  **temporal-ui**: Interfaz de usuario para monitorizar los flujos (Puerto 8085).
8.  **spotlight**: Herramienta de depuración incluida en el stack.

## 🔑 Configuración del Servidor (OCI)

### Configuración Dinámica de Temporal

Para que Temporal arranque correctamente, se inyectó un archivo de configuración en el host:

- **Ruta:** `/data/coolify/applications/TU_APP_ID/dynamicconfig/development-sql.yaml`
- **Contenido:** Define límites de concurrencia para evitar bloqueos en el inicio.

### Red de Redes

Todos los servicios están conectados a la red **`coolify`** (externa) para que Traefik pueda gestionar el SSL de `postiz.tu-dominio.com` automáticamente.

## 📦 Identificadores de Recursos (UUIDs)

- **Aplicación (postiz-ultimate):** `TU_APP_ID`
- **Despliegue Final:** `TU_DEPLOYMENT_ID`

## 🔧 Gestión y Mantenimiento

### Ver logs de la aplicación

```bash
ssh -i ~/.ssh/tu-clave.key ubuntu@TU_IP_PUBLICA "docker logs postiz-TU_APP_ID-130808666463 --tail 100"
```

### Reiniciar un servicio específico

```bash
ssh -i ~/.ssh/tu-clave.key ubuntu@TU_IP_PUBLICA "docker restart postiz-TU_APP_ID-130808666463"
```

## 📁 Almacenamiento

Actualmente configurado como **`local`** en `/uploads`.
Para cambiar a **OCI Object Storage**, consulta el archivo [`OCI_OBJECT_STORAGE.md`](./OCI_OBJECT_STORAGE.md).
