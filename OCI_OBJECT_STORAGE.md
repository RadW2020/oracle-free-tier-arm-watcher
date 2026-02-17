# 📦 OCI Object Storage Setup for Postiz

Este documento detalla la configuración del almacenamiento de medios para Postiz usando **OCI Object Storage**.

## ✅ Infraestructura Creada

### Bucket de Object Storage

- **Nombre:** `postiz-media`
- **Namespace:** `TU_NAMESPACE`
- **Región:** `eu-madrid-3`
- **Tipo de Acceso:** Privado (sin acceso público directo)
- **Compatibilidad:** S3 API

### Customer Secret Keys (S3-Compatible)

Se generaron credenciales de acceso compatibles con S3 para que Postiz pueda subir y gestionar archivos:

```env
AWS_ACCESS_KEY_ID=tu_access_key_id
AWS_SECRET_ACCESS_KEY=tu_secret_access_key
```

## 🔧 Configuración en Postiz

Las siguientes variables de entorno fueron configuradas en la aplicación Postiz en Coolify:

| Variable                | Valor                                                                                |
| ----------------------- | ------------------------------------------------------------------------------------ |
| `STORAGE_PROVIDER`      | `s3`                                                                                 |
| `AWS_ACCESS_KEY_ID`     | `tu_access_key_id`                                                                   |
| `AWS_SECRET_ACCESS_KEY` | `tu_secret_access_key`                                                               |
| `AWS_ENDPOINT`          | `https://TU_NAMESPACE.compat.objectstorage.eu-madrid-3.oraclecloud.com`              |
| `AWS_BUCKET`            | `postiz-media`                                                                       |
| `AWS_REGION`            | `eu-madrid-3`                                                                        |
| `AWS_PUBLIC_URL`        | `https://objectstorage.eu-madrid-3.oraclecloud.com/n/TU_NAMESPACE/b/postiz-media/o/` |
| `AWS_FORCE_PATH_STYLE`  | `true`                                                                               |

## 💰 Límites del Free Tier

OCI Object Storage ofrece generosamente:

- ✅ **20GB de almacenamiento** (vs 10GB de Cloudflare R2)
- ✅ **10GB de egreso/mes** (transferencia de datos salientes)
- ✅ **50,000 solicitudes PUT/POST/LIST por mes**
- ✅ **50,000 solicitudes GET por mes**

## 🔐 Seguridad

- Las credenciales están almacenadas en el `.env` del repositorio (no versionado en Git)
- El bucket es **privado** - solo accesible mediante las Customer Secret Keys
- Los archivos se sirven a través de URLs firmadas generadas por Postiz

## 🛠️ Gestión del Bucket

### Ver contenido del bucket

```bash
# Requiere OCI CLI configurado
oci os object list --bucket-name postiz-media --namespace TU_NAMESPACE
```

### Eliminar archivos antiguos (si es necesario)

```bash
# Ejemplo: eliminar archivos de más de 90 días
oci os object bulk-delete --bucket-name postiz-media --namespace TU_NAMESPACE --older-than 90
```

## 📝 Notas Importantes

1. **Reinicio de Postiz:** Después de configurar el almacenamiento, es recomendable reiniciar la aplicación para que tome los cambios.
2. **Migración de archivos locales:** Si ya habías subido archivos con `STORAGE_PROVIDER=local`, necesitarás migrarlos manualmente al bucket.
3. **Monitoreo de uso:** Revisa periódicamente el uso del bucket en la consola de OCI para asegurarte de no exceder los límites del Free Tier.

## 🔄 Rollback (si es necesario)

Si necesitas volver al almacenamiento local:

1. Elimina las variables `AWS_*` de Postiz en Coolify
2. Configura:
   ```env
   STORAGE_PROVIDER=local
   UPLOAD_DIRECTORY=/uploads
   ```
3. Reinicia la aplicación
