import { Frequency, UrlAssertionBuilder, UrlMonitor } from 'checkly/constructs'
import { strongCoreGroup } from '../groups'
import { LOCATIONS } from '../retries'

/**
 * Renderizado bajo demanda de Strong Core: la página de historial.
 *
 * Existe porque `strong-core-web` vigila `/`, que es un prerender cacheado y
 * por tanto puede devolver 200 con la ruta dinámica de la app rota.
 * `/historial` no se cachea (`cache-control: private, no-cache, no-store`):
 * cada petición la renderiza el servidor, así que su 200 sí demuestra que el
 * camino dinámico funciona.
 *
 * Se sondea sin sesión, que es lo único que puede hacer un check sin
 * credenciales: devuelve el shell de la página, no el historial de nadie.
 * Cubre "el servidor renderiza", no "los datos del usuario están ahí".
 *
 * UrlMonitor porque la assertion útil es el status: la página es HTML y no
 * expone un contrato sobre el que afirmar. Coste en API runs: 0.
 */
new UrlMonitor('strong-core-historial', {
  name: 'Strong Core Historial (SSR)',
  request: {
    url: 'https://strong-core-after-wod.uliber.com/historial',
    ipFamily: 'IPv4',
    assertions: [
      UrlAssertionBuilder.statusCode().equals(200),
    ],
  },
  degradedResponseTime: 5000,
  maxResponseTime: 20000,
  activated: true,
  muted: false,
  shouldFail: false,
  tags: [
    'strong-core',
  ],
  frequency: Frequency.EVERY_1H,
  // Las localizaciones y la estrategia de reintento las fija el grupo
  // (src/groups.ts -> src/retries.ts): declararlas aquí no tiene efecto.
  group: strongCoreGroup,
  runParallel: false,
})
