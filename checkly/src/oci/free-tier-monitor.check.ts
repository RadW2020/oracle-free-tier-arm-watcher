import { ApiCheck, AssertionBuilder, Frequency, RetryStrategyBuilder } from 'checkly/constructs'
import { raulEmailAlert } from '../alert-channels'
import { LOCATIONS } from '../retries'
import { standardEscalation } from '../escalation'

new ApiCheck('oracle-free-tier-monitor-lAoPm1wX', {
  name: 'Oracle Free Tier Monitor',
  description: 'Cuota, no latencia. Falla si el watcher no responde o si `/status` deja de traer `maxUsagePercentage`, que es el contrato del que dependen estos checks. Un aviso de degradación aquí sólo dice que `/status` tardó más de 15 s (llama a la API de OCI en cada run); no que el host esté lento.',
  request: {
    url: '{{ORACLE_MONITOR_URL}}/status',
    method: 'GET',
    ipFamily: 'IPv4',
    headers: [
      {
        key: 'X-API-Key',
        value: '{{ORACLE_MONITOR_API_KEY}}',
      },
    ],
    assertions: [
      AssertionBuilder.statusCode().equals(200),
      AssertionBuilder.jsonBody('$.maxUsagePercentage').lessThan('101'),
    ],
  },
  // 15 s y no 5, por lo mismo que los dos de bandwidth: este check vigila la
  // cuota, no la latencia, y /status llama a OCI en cada run (p99 ~2,7 s en
  // una semana normal). El 11/10/2026 una pérdida de paquetes en el camino
  // hacia Madrid ralentizó todo el host 25 min y los checks de cuota mandaron
  // avisos de degradación con el uso al 0 %. La lentitud del host ya la
  // avisan los checks de servicio; aquí sólo confundía.
  degradedResponseTime: 15000,
  maxResponseTime: 20000,
  activated: true,
  muted: false,
  shouldFail: false,
  locations: LOCATIONS,
  // 3 h y no 1 h: este check pega al servicio Go, que a su vez llama a la
  // API de OCI. A 1 h eran 24 consultas diarias contra Oracle en vez de 8,
  // y no compensa arriesgar rate limits en la API que vigila la factura.
  frequency: Frequency.EVERY_3H,
  alertChannels: [
    raulEmailAlert,
  ],
  alertEscalationPolicy: standardEscalation,
  retryStrategy: RetryStrategyBuilder.fixedStrategy({
    baseBackoffSeconds: 30,
    maxRetries: 2,
    maxDurationSeconds: 600,
    sameRegion: false,
  }),
  runParallel: false,
})
