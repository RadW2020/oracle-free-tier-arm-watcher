import { AlertEscalationBuilder, Frequency, HeartbeatMonitor, RetryStrategyBuilder } from 'checkly/constructs'
import { raulEmailAlert } from '../alert-channels'

new HeartbeatMonitor('ciaobox-weekly-close-heartbeat-6chrZXYQ', {
  name: 'Ciaobox weekly-close heartbeat',
  period: 7,
  periodUnit: 'days',
  grace: 24,
  graceUnit: 'hours',
  activated: true,
  muted: false,
  shouldFail: false,
  tags: [
    'ciaobox',
    'cron',
    'heartbeat',
  ],
  frequency: Frequency.EVERY_10M,
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
  retryStrategy: RetryStrategyBuilder.noRetries(),
  runParallel: false,
})
