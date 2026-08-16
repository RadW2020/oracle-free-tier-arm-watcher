import { Frequency, RetryStrategyBuilder, UrlAssertionBuilder, UrlMonitor } from 'checkly/constructs'
import { eduSchedulerGroup } from '../groups'
import { standardEscalation } from '../escalation'

/**
 * Up/down de la web de EduScheduler.
 *
 * Era un ApiCheck cuya única assertion era `statusCode == 200`, es decir,
 * exactamente lo que hace un UrlMonitor. A 30 min consumía 1.460 API runs
 * al mes (de 10.000 del plan). Como UrlMonitor cuesta 0 runs: los uptime
 * monitors se facturan por unidad, no por ejecución.
 *
 * Si algún día hace falta afirmar sobre el body o las cabeceras, hay que
 * volver a ApiCheck — los UrlMonitor sólo admiten assertions de status.
 */
new UrlMonitor('edu-scheduler-web-ui-bzUJVMx7', {
  name: 'EduScheduler Web UI',
  request: {
    url: 'https://eduweb.uliber.com',
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
    'eduscheduler',
  ],
  frequency: Frequency.EVERY_30M,
  group: eduSchedulerGroup,
  alertEscalationPolicy: standardEscalation,
  // Los uptime monitors no admiten estrategias de reintento con
  // varios intentos en este plan: sólo un reintento único.
  retryStrategy: RetryStrategyBuilder.singleRetry({
    baseBackoffSeconds: 30,
    sameRegion: true,
  }),
  runParallel: false,
})
