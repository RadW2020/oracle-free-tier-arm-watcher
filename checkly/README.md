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
| `CIAOBOX_CRON_TOKEN` | `ciaobox/weekly-close-trigger` | ❌ **hay que crearla** |

Los checks de OCI y el trigger de Ciaobox tenían el secreto escrito en claro en
Checkly (los creó un script bash con la clave literal). Aquí se referencian por
variable. Para OCI las variables ya existen y contienen exactamente el mismo
valor, así que el cambio es funcionalmente nulo.

`CIAOBOX_CRON_TOKEN` todavía no existe: hay que crearla **antes del primer
deploy** con el token que hoy lleva la cabecera `Authorization` de ese check
(el valor va sin el prefijo `Bearer `, que ya está en el código):

```bash
set -a && . ../.env && set +a
curl -X POST "https://api.checklyhq.com/v1/variables" \
  -H "Authorization: Bearer $CHECKLY_API_KEY" \
  -H "X-Checkly-Account: $CHECKLY_ACCOUNT_ID" \
  -H "Content-Type: application/json" \
  -d '{"key":"CIAOBOX_CRON_TOKEN","value":"<TOKEN>","locked":true}'
```

## Estado del import: hay un plan pendiente

Los 6 checks de Shogunito ya eran nativos del CLI. Los otros 8 checks, los 2
grupos, el canal de email y la status page se crearon por API o a mano, así que
no tenían `logicalId` y el CLI no los reconocía. Un `deploy` los habría
duplicado.

Para adoptarlos se creó un **plan de importación** (`checkly import`), del que
salió el código en `src/oci`, `src/ciaobox`, `src/eduscheduler`,
`src/status-page`, `src/groups.ts` y `src/alert-channels.ts`.

**El plan está creado pero no aplicado.** Los recursos todavía no están
enlazados al proyecto, así que **desplegar ahora crearía duplicados**. Los pasos
que faltan:

```bash
cd checkly
set -a && . ../.env && set +a

npx checkly import apply     # enlaza los recursos existentes con este código
npx checkly deploy --preview # diff: debería salir sin cambios destructivos
npx checkly import commit    # cierra la sesión de import
npx checkly deploy           # a partir de aquí, este repo manda
```

`apply` es reversible con `npx checkly import cancel`. `commit` no: a partir de
ahí, borrar un construct de este repo borra el recurso en Checkly en el
siguiente deploy.

## Pendiente

- **El dashboard `shogun-status` (id 871789) no está aquí.** `checkly import` no
  generó construct para él; se sigue gestionando desde el dashboard web. Además
  filtra por tags `critical` y `cloudflare`, que ningún check tiene, así que hoy
  se ve vacío.
- **`OCI Bandwidth WARNING (50% = 5TB)` no avisa al 50 %.** Su aserción es
  `< 70`, igual que la del CRITICAL, así que ambos disparan a la vez. El código
  refleja el estado real; corregirlo a `< 50` es un cambio de comportamiento y
  se ha dejado como decisión aparte.
- **Variables de cuenta sin `locked`.** `ORACLE_MONITOR_API_KEY` y
  `CHECKLY_TEST_USER_PASSWORD` se leen en claro desde el dashboard y la API.
- **`CHECKLY_TEST_USER_PASSWORD` vale literalmente `"null"`**, así que cualquier
  spec que dependa de ella no puede autenticarse.
