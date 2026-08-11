import { AlertEscalationBuilder, ApiCheck, AssertionBuilder, Frequency, RetryStrategyBuilder } from 'checkly/constructs'
import { raulEmailAlert } from '../alert-channels'

new ApiCheck('oci-bandwidth-critical-70-7-tb-NTGCJelN', {
  name: 'OCI Bandwidth CRITICAL (70% = 7TB)',
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
  alertEscalationPolicy: AlertEscalationBuilder.runBasedEscalation(1, {
    amount: 0,
    interval: 5,
  }, {
    enabled: false,
    percentage: 10,
  }),
  retryStrategy: RetryStrategyBuilder.fixedStrategy({
    baseBackoffSeconds: 30,
    maxRetries: 2,
    maxDurationSeconds: 600,
    sameRegion: true,
  }),
  runParallel: false,
})
