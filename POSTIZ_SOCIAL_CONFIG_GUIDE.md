# Guía de Configuración de Canales Sociales en Postiz

Esta guía detalla cómo configurar **YouTube** y **TikTok** en tu instancia de Postiz (`https://postiz.tu-dominio.com`).

> **Nota para múltiples cuentas:** Postiz utiliza un único set de credenciales de API por plataforma a nivel de servidor (variables de entorno). Esto significa que usarás **la misma aplicación de desarrollador** para conectar tanto tu cuenta en inglés como la de español.

---

## 📽️ Configuración de YouTube (Google Cloud)

Para conectar tus dos canales de YouTube, solo necesitas crear un proyecto en Google Cloud.

### 1. Crear el Proyecto y Activar APIs

1. Accede a [Google Cloud Console](https://console.cloud.google.com/).
2. Crea un nuevo proyecto llamado `Postiz-Social`.
3. En el buscador superior, busca y **habilita** las siguientes APIs:
   - **YouTube Data API v3**
   - **YouTube Analytics API**
   - **YouTube Reporting API**

### 2. Configurar el "OAuth Consent Screen"

1. Ve a **APIs & Services > OAuth consent screen**.
2. Selecciona **User Type: External**.
3. Rellena los datos básicos (App name, Email).
4. **Scopes (Alcances):** Añade `/auth/youtube.upload` y `/auth/youtube.readonly`.
5. **Test Users:** ¡IMPORTANTE! Como tu app no estará verificada por Google inicialmente, debes añadir los correos electrónicos de **tus dos cuentas de YouTube** (la de español y la de inglés) en la sección "Test Users".

### 3. Crear Credenciales (Client ID)

1. Ve a **Credentials > Create Credentials > OAuth client ID**.
2. Application type: **Web application**.
3. **Authorized redirect URIs:** Añade exactamente:
   `https://postiz.tu-dominio.com/integrations/social/youtube`
4. Copia el **Client ID** y el **Client Secret**.

---

## 🎵 Configuración de TikTok (TikTok for Developers)

Para **Fabulous Universe**, usaremos una única App de TikTok para gestionar ambas cuentas (EN/ES).

### 1. Crear la Aplicación en TikTok

1. Ve a [TikTok for Developers](https://developers.tiktok.com/).
2. Asegúrate de estar en el modo **Production** (no Sandbox).
3. Haz clic en **"Connect an App"**.
4. **App Name:** `Fabulous Universe Manager`
5. **Categoría:** `Entertainment`.

### 2. Añadir Productos (Essential)

Debes añadir estos productos en este orden:

1. **Login Kit:** Permite la conexión inicial.
2. **Content Posting API:** Permite subir vídeos. Una vez añadido, entra en su configuración y asegúrate de que **"Direct Post"** esté seleccionado.

### 3. Rellenar Información del Proyecto (App Details)

Usa estos textos precisos para Fabulous Universe:

- **Description (en inglés):**
  > "This application is the central management hub for Fabulous Universe, a multimedia project that creates and distributes high-quality content across multiple languages. We use the TikTok Login Kit and Content Posting API to manage our official brand channels: Fabulous Universe (English) and Fabulous Universe (Spanish). It allows our team to schedule and publish video content directly from our internal dashboard at postiz.tu-dominio.com."
- **Terms of Service URL:** `https://tu-dominio.com`
- **Privacy Policy URL:** `https://tu-dominio.com`
- **App Icon:** Sube el logo de Fabulous Universe (min. 480x480px).

### 4. Configurar Scopes (Permisos)

En la sección de **Scopes**, asegúrate de marcar los siguientes (son obligatorios para que Postiz pueda publicar):

- **`user.info.basic`**: Para mostrar tu perfil en el dashboard.
- **`video.upload`**: Permite enviar el archivo de vídeo.
- **`video.publish`**: Permite que el vídeo sea público en tu feed.
- **`video.list`**: (Recomendado) Para verificar el estado de tus publicaciones.

### 5. Configurar Redirect URI (Crucial)

Dentro de la configuración del **Login Kit**:

- **Redirect URI:** `https://postiz.tu-dominio.com/integrations/social/tiktok`

### 6. Verificación de Dominio

En la sección de verificación, elige **DNS Record**:

1. Escribe `tu-dominio.com`.
2. Copia el valor TXT (ej: `tiktok-verify=123...`).
3. Añádelo a tus DNS del dominio `tu-dominio.com`.

### 7. Enviar a Revisión (App Submission)

Cuando te pida la explicación técnica (Explanation):

> "Our application uses the Login Kit for secure authentication. We require scopes like video.upload and video.publish to enable our internal team to schedule and publish original videos for Fabulous Universe. The flow involves: User login via TikTok auth -> Video upload to our scheduler -> Automated publishing via Content Posting API at the scheduled time."

**Demo Video:** Graba un vídeo corto (<50MB) de tu Postiz mostrando cómo intentas añadir el canal de TikTok.

### 8. Añadir Cuentas de Prueba (Para usarlo YA)

Para conectar tus cuentas hoy mismo sin esperar la revisión:

1. Busca la sección **"Test Accounts"**.
2. Añade los handles: `@fabulous_universe_en` y `@fabulous_universe_es` (o los que uses).
3. **Acepta la invitación** en la app de TikTok del móvil (en Notificaciones).

---

## 🚀 Paso Final: Configurar en Coolify

Una vez tengas las credenciales, debes volcarlas en las variables de entorno de tu aplicación en Coolify.

### 1. Variables a añadir:

| Variable                | Origen                     |
| :---------------------- | :------------------------- |
| `YOUTUBE_CLIENT_ID`     | Google OAuth Client ID     |
| `YOUTUBE_CLIENT_SECRET` | Google OAuth Client Secret |
| `TIKTOK_CLIENT_ID`      | TikTok Client Key          |
| `TIKTOK_CLIENT_SECRET`  | TikTok Client Secret       |

### 2. Conectar las cuentas en Postiz:

1. Una vez desplegada la app con las variables, entra en `https://postiz.tu-dominio.com`.
2. Ve a **Add Channel > YouTube**.
3. Inicia sesión con tu cuenta de **YouTube Español**. Autoriza los permisos.
4. Repite el proceso: **Add Channel > YouTube**.
5. Inicia sesión con tu cuenta de **YouTube Inglés**.
6. Haz lo mismo con **TikTok** para ambas cuentas.

¡Listo! Ahora verás los 4 canales en tu dashboard de Postiz y podrás programar contenido de forma independiente para cada uno.
