import { ApiCheck, AssertionBuilder, Frequency, RetryStrategyBuilder } from 'checkly/constructs'
import { raulEmailAlert } from '../alert-channels'
import { LOCATIONS } from '../retries'
import { standardEscalation } from '../escalation'

new ApiCheck('oci-bandwidth-critical-70-7-tb-NTGCJelN', {
  name: 'OCI Bandwidth CRITICAL (70% = 7TB)',
  description: 'Cuota, no latencia. Falla si el egress del mes llega al **70 %** de los 10 TB gratis (`$.usage.bandwidth.percentage`), o si el watcher no responde o no está configurado. Un aviso de degradación aquí sólo dice que `/usage` tardó más de 15 s (hace 13 + N llamadas a OCI); no que el host esté lento ni que se esté gastando ancho de banda.',
  request: {
    url: '{{ORACLE_MONITOR_URL}}/usage',
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
      AssertionBuilder.jsonBody('$.configured').equals('true'),
      AssertionBuilder.jsonBody('$.usage.bandwidth.percentage').lessThan('70'),
    ],
  },
  // 15 s y no 5: este check vigila la cuota, no la latencia. /usage hace
  // 13 + N llamadas a OCI en cada run (p99 ~4 s en una semana normal), así
  // que con 5 s cualquier tirón de red acababa en un email de «Degradation
  // alert for OCI Bandwidth CRITICAL» con el uso al 0 %: pasó el 11/10/2026,
  // cuando una pérdida de paquetes en el camino hacia Madrid ralentizó todo el
  // host durante 25 min. La lentitud del host ya la avisan los checks de
  // servicio; aquí sólo confundía.
  degradedResponseTime: 15000,
  maxResponseTime: 20000,
  activated: true,
  muted: false,
  shouldFail: false,
  locations: LOCATIONS,
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
