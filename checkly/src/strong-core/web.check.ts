import { Frequency, RetryStrategyBuilder, UrlAssertionBuilder, UrlMonitor } from 'checkly/constructs'
import { strongCoreGroup } from '../groups'

/**
 * Up/down de la PWA de Strong Core After WOD.
 *
 * UrlMonitor y no ApiCheck porque aquí sólo interesa el up/down: los uptime
 * monitors se facturan por unidad (10 en el plan) en vez de consumir la
 * cuota de 10.000 API runs/mes, así que su frecuencia es gratis.
 *
 * Ojo con lo que este check NO prueba. `/` es un prerender estático que se
 * sirve desde la caché de Next (`x-nextjs-prerender: 1`,
 * `x-nextjs-cache: HIT`, `s-maxage=31536000`): su 200 demuestra que Traefik
 * enruta y que el proceso de Node está vivo, pero no que la app pueda
 * renderizar bajo demanda ni hablar con su base de datos. Eso lo cubren
 * `historial` y `auth-providers`.
 */
new UrlMonitor('strong-core-web', {
  name: 'Strong Core Web',
  request: {
    url: 'https://strong-core-after-wod.uliber.com/',
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
  locations: [
    'eu-central-1',
  ],
  tags: [
    'strong-core',
  ],
  frequency: Frequency.EVERY_30M,
  group: strongCoreGroup,
  // Los uptime monitors no admiten estrategias de reintento con
  // varios intentos en este plan: sólo un reintento único.
  retryStrategy: RetryStrategyBuilder.singleRetry({
    baseBackoffSeconds: 30,
    sameRegion: true,
  }),
  runParallel: false,
})
