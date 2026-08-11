# Checkly — configuration as code

Fuente de verdad de **toda la cuenta de Checkly** de Uliber & Co
(`d6455d4f-64b9-449f-a1cd-0456e2092597`). Antes de esto, la configuración vivía
repartida entre el repo de Shogunito (6 checks como código), unos scripts bash
en la raíz de este repo (`setup-checkly.sh`, `update-checkly.sh`, …) y el
dashboard de Checkly. Ahora vive aquí.

## ⚠️ El `logicalId` del proyecto no se toca

`checkly.config.ts` declara `logicalId: 'shogunito-project'`. Es el nombre
heredado de cuando el proyecto vivía en el repo de Shogunito, y es la identidad
del proyecto en Checkly. Cambiarlo haría que el CLI tratase los recursos
existentes como nuevos: crearía duplicados y dejaría huérfanos los originales,
perdiendo su historial de runs y sus métricas. `projectName` sí es cosmético.

## Estructura

```
checkly/
├── checkly.config.ts          Proyecto: logicalId, checkMatch, runLocation
├── src/
│   ├── alert-channels.ts      Canal de email → raul@uliber.com
│   ├── groups.ts              Grupos Shogunito y EduScheduler
│   ├── oci/                   3 checks del Free Tier de Oracle
│   ├── shogunito/             4 API + 1 browser + 1 multistep (+ specs Playwright)
│   ├── ciaobox/               2 API + 1 heartbeat
│   ├── eduscheduler/          2 API
│   └── status-page/           Status page pública + sus 11 servicios
└── SECURITY.md                Cómo tratar secretos en los checks
```

Los ficheros `*.check.ts` bajo `src/` se cargan por `checkMatch`. Los que no
llevan ese sufijo (`alert-channels.ts`, `groups.ts`, `status-page/services.ts`)
se cargan porque otros los importan.

Las specs de Playwright (`src/shogunito/*.spec.ts`) las referencian explícitamente
sus constructs. Por eso `browserChecks.testMatch` apunta a `*.browser.spec.ts`
y no a `*.spec.ts`: si coincidieran, se desplegarían dos veces.

## Autenticación

El CLI lee `CHECKLY_API_KEY` y `CHECKLY_ACCOUNT_ID`, que ya están en el `.env`
de la raíz del repo (fuera de git):

```bash
cd checkly
set -a && . ../.env && set +a
npm run validate
```

Como alternativa, `npx checkly login`.

## Secretos

**Nunca se escribe un secreto en estos ficheros.** Se referencian variables de
entorno de la cuenta de Checkly con `{{NOMBRE}}`:

| Variable | Usada en | Estado |
|---|---|---|
| `ORACLE_MONITOR_URL` | los 3 checks de OCI | ✅ existe |
| `ORACLE_MONITOR_API_KEY` | los 3 checks de OCI | ✅ existe |
| `API_URL`, `WEB_URL`, `MINIO_URL` | checks de Shogunito | ✅ existen |
| `CIAOBOX_CRON_TOKEN` | `ciaobox/weekly-close-trigger` | ✅ existe (`locked`) |

Los checks de OCI y el trigger de Ciaobox tenían el secreto escrito en claro en
Checkly (los creó un script bash con la clave literal). Aquí se referencian por
variable. Para OCI las variables ya existen y contienen exactamente el mismo
valor, así que el cambio es funcionalmente nulo.

`CIAOBOX_CRON_TOKEN` se creó extrayendo el token de la cabecera `Authorization`
que el check ya tenía en Checkly, sin el prefijo `Bearer ` (que vive en el
código). Se verificó que `'Bearer ' + variable` reconstruye exactamente la
cabecera original, de modo que el deploy no cambia el comportamiento del check.

⚠️ **`locked` protege menos de lo que parece.** Oculta el valor en la UI del
dashboard, pero `GET /v1/variables` con una API key de cuenta lo sigue
devolviendo en claro. Sirve para evitar lecturas casuales, no como control de
acceso: quien tenga una API key de la cuenta puede leer cualquier variable.

Si alguna vez hay que rotarla:

```bash
set -a && . ../.env && set +a
curl -X PUT "https://api.checklyhq.com/v1/variables/CIAOBOX_CRON_TOKEN" \
  -H "Authorization: Bearer $CHECKLY_API_KEY" \
  -H "X-Checkly-Account: $CHECKLY_ACCOUNT_ID" \
  -H "Content-Type: application/json" \
  -d '{"value":"<TOKEN NUEVO>","locked":true}'
```

## Estado: este repo ya manda sobre la cuenta

Los 6 checks de Shogunito ya eran nativos del CLI. Los otros 8 checks, los 2
grupos, el canal de email y la status page se crearon por API o a mano, así que
no tenían `logicalId` y el CLI no los reconocía. Un `deploy` los habría
duplicado.

Para adoptarlos se usó `checkly import` (plan → apply → commit), del que salió
el código en `src/oci`, `src/ciaobox`, `src/eduscheduler`, `src/status-page`,
`src/groups.ts` y `src/alert-channels.ts`.

El plan `c1d58198-dcf3-4446-a548-8ff2d8d2580b` se aplicó y commiteó el
2026-08-11, con un deploy en medio para verificar antes de perder el failsafe.
Los 14 checks tienen ya `logicalId`, así que **los 29 recursos del proyecto
están gestionados desde aquí**. El diff del deploy salió con los 29 como
*Update and Unchanged*, sin *Create* ni *Delete*: cero duplicados, cero
borrados, IDs e historial intactos, y el ping token del heartbeat sin cambiar.

⚠️ **A partir de ahora, borrar un construct de este repo borra el recurso en
Checkly en el siguiente deploy.** No hay red de seguridad.

Flujo normal de trabajo:

```bash
cd checkly
set -a && . ../.env && set +a

npx checkly validate                              # sintaxis y tipos
npx checkly test --grep "<nombre del check>"      # ejecuta contra el endpoint real
npx checkly deploy --preview                      # diff antes de aplicar
npx checkly deploy
```

Cuidado con `checkly test` sobre `ciaobox weekly-close trigger`: es un POST
contra `/api/cron/weekly-close` y dispara una acción de negocio real.

## Pendiente

- **El dashboard `shogun-status` (id 871789) no está aquí.** `checkly import` no
  generó construct para él; se sigue gestionando desde el dashboard web. Además
  filtra por tags `critical` y `cloudflare`, que ningún check tiene, así que hoy
  se ve vacío.
- **Los dos avisos de bandwidth están escalonados**, ya sí: WARNING a `< 50`
  (5 TB) y CRITICAL a `< 70` (7 TB). Venían los dos con `< 70`, de modo que
  disparaban a la vez y el aviso temprano no existía. Arreglado y desplegado con
  el uso al 0 %, así que el cambio no generó ninguna alerta.
- **`CHECKLY_TEST_USER_PASSWORD` no tiene valor**: la API la devuelve como
  `null`, no como cadena vacía ni como texto. Cualquier spec que dependa de ella
  no puede autenticarse.
- **Las variables de URL siguen sin `locked`**, que es lo razonable: no son
  secretos. Las dos que sí lo son (`ORACLE_MONITOR_API_KEY`,
  `CIAOBOX_CRON_TOKEN`) están marcadas, con la limitación de arriba.
