import { ApiCheck, AssertionBuilder, Frequency, RetryStrategyBuilder } from 'checkly/constructs'
import { raulEmailAlert } from '../alert-channels'
import { standardEscalation } from '../escalation'

new ApiCheck('oracle-free-tier-monitor-lAoPm1wX', {
  name: 'Oracle Free Tier Monitor',
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
  degradedResponseTime: 5000,
  maxResponseTime: 20000,
  activated: true,
  muted: false,
  shouldFail: false,
  locations: [
    'eu-central-1',
  ],
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
    sameRegion: true,
  }),
  runParallel: false,
})
