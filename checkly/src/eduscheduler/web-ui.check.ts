import { AlertEscalationBuilder, ApiCheck, AssertionBuilder, Frequency, RetryStrategyBuilder } from 'checkly/constructs'
import { eduSchedulerGroup } from '../groups'

new ApiCheck('edu-scheduler-web-ui-bzUJVMx7', {
  name: 'EduScheduler Web UI',
  request: {
    url: 'https://eduweb.uliber.com',
    method: 'GET',
    ipFamily: 'IPv4',
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
    'eduscheduler',
  ],
  frequency: Frequency.EVERY_30M,
  group: eduSchedulerGroup,
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
