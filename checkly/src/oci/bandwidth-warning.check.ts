import { ApiCheck, AssertionBuilder, Frequency, RetryStrategyBuilder } from 'checkly/constructs'
import { raulEmailAlert } from '../alert-channels'
import { standardEscalation } from '../escalation'

new ApiCheck('oci-bandwidth-warning-50-5-tb-mHqJeYhB', {
  name: 'OCI Bandwidth WARNING (50% = 5TB)',
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
      // 50%, no 70: con 70 este check disparaba a la vez que el CRITICAL y el
      // aviso temprano de 5 TB nunca llegaba.
      AssertionBuilder.jsonBody('$.usage.bandwidth.percentage').lessThan('50'),
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
  frequency: Frequency.EVERY_12H,
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
