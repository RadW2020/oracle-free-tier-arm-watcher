import { Frequency, RetryStrategyBuilder, UrlAssertionBuilder, UrlMonitor } from 'checkly/constructs'
import { aidraGroup } from '../groups'

/**
 * Dashboard público de Grafana de AIDRA (el "Dashboard of the Month").
 *
 * Es un UrlMonitor y no un ApiCheck a propósito: aquí sólo interesa el
 * up/down, y los uptime monitors se facturan por unidad (10 en el plan)
 * en vez de consumir la cuota de 10.000 API runs/mes. Coste en runs: 0.
 *
 * Se apunta a la URL final del public dashboard, no a `aidra.uliber.com/`,
 * que es un 302 de Traefik hacia aquí. Vigilando el destino se cubre
 * también que Grafana sirva el dashboard, no sólo que redirija.
 */
new UrlMonitor('aidra-grafana-dashboard', {
  name: 'AIDRA Dashboard (Grafana)',
  request: {
    url: 'https://aidra.uliber.com/public-dashboards/f86d408b3b744d149051330e131a47c8',
    ipFamily: 'IPv4',
    followRedirects: true,
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
    'aidra',
  ],
  frequency: Frequency.EVERY_1H,
  group: aidraGroup,
  // Los uptime monitors no admiten estrategias de reintento con
  // varios intentos en este plan: sólo un reintento único.
  retryStrategy: RetryStrategyBuilder.singleRetry({
    baseBackoffSeconds: 30,
    sameRegion: true,
  }),
  runParallel: false,
})
