import { AlertEscalationBuilder, ApiCheck, AssertionBuilder, Frequency, RetryStrategyBuilder } from 'checkly/constructs'
import { raulEmailAlert } from '../alert-channels'

new ApiCheck('ciaobox-weekly-close-trigger-ksR0Xopp', {
  name: 'ciaobox weekly-close trigger',
  request: {
    url: 'https://ciaobox.uliber.com/api/cron/weekly-close',
    method: 'POST',
    ipFamily: 'IPv4',
    headers: [
      {
        key: 'Authorization',
        value: 'Bearer {{CIAOBOX_CRON_TOKEN}}',
        locked: true,
      },
    ],
    assertions: [
      AssertionBuilder.statusCode().equals(200),
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
    'ciaobox',
    'cron',
  ],
  frequency: Frequency.EVERY_1H,
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
  retryStrategy: RetryStrategyBuilder.linearStrategy({
    baseBackoffSeconds: 60,
    maxRetries: 3,
    maxDurationSeconds: 600,
    sameRegion: true,
  }),
  runParallel: false,
})
