import { ApiCheck, AssertionBuilder, Frequency, RetryStrategyBuilder } from 'checkly/constructs'
import { strongCoreGroup } from '../groups'

/**
 * Que se pueda entrar en Strong Core, no sólo que la web cargue.
 *
 * La app no tiene endpoint de health, así que el contrato más útil que
 * expone sin credenciales es `/api/auth/providers` de Auth.js. Es una ruta
 * dinámica: su 200 demuestra que Auth.js arrancó con su configuración, algo
 * que un 200 en `/` no dice.
 *
 * El login es sólo por magic link (proveedor `nodemailer`, sin OAuth ni
 * contraseña), y de ahí las dos assertions sobre el body:
 *
 * - `$.nodemailer.id`: si el proveedor desaparece del payload, no hay
 *   ninguna forma de entrar en la app aunque todo devuelva 200.
 * - `$.nodemailer.callbackUrl`: Auth.js compone esa URL con su base
 *   configurada (`AUTH_URL`/`NEXTAUTH_URL`). Si se despliega con la base
 *   equivocada, los enlaces del correo apuntan a otro host y el login se
 *   rompe en silencio — nada falla, simplemente los magic links no llevan a
 *   ninguna parte. Afirmar la URL completa es lo que lo detecta.
 *
 * ApiCheck y no UrlMonitor porque son precisamente esas assertions sobre el
 * JSON lo que aporta: un UrlMonitor sólo sabe de status codes. Coste: 730
 * API runs/mes a 1 h.
 *
 * Se usa GET explícito: este endpoint responde 400 a HEAD.
 */
new ApiCheck('strong-core-auth-providers', {
  name: 'Strong Core Auth Providers',
  request: {
    url: 'https://strong-core-after-wod.uliber.com/api/auth/providers',
    method: 'GET',
    ipFamily: 'IPv4',
    assertions: [
      AssertionBuilder.statusCode().equals(200),
      AssertionBuilder.jsonBody('$.nodemailer.id').equals('nodemailer'),
      AssertionBuilder.jsonBody('$.nodemailer.callbackUrl').equals(
        'https://strong-core-after-wod.uliber.com/api/auth/callback/nodemailer',
      ),
    ],
  },
  degradedResponseTime: 5000,
  maxResponseTime: 20000,
  activated: true,
  muted: false,
  shouldFail: false,
  locations: [
    'eu-central-1',
  ],
  tags: [
    'strong-core',
  ],
  frequency: Frequency.EVERY_1H,
  group: strongCoreGroup,
  retryStrategy: RetryStrategyBuilder.fixedStrategy({
    baseBackoffSeconds: 30,
    maxRetries: 2,
    maxDurationSeconds: 600,
    sameRegion: true,
  }),
  runParallel: false,
})
