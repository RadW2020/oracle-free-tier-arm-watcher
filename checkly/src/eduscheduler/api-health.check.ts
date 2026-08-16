import { ApiCheck, AssertionBuilder, Frequency, RetryStrategyBuilder } from 'checkly/constructs'
import { eduSchedulerGroup } from '../groups'
import { standardEscalation } from '../escalation'

new ApiCheck('edu-scheduler-api-health-zJ9hn4l5', {
  name: 'EduScheduler API Health',
  request: {
    url: 'https://eduapi.uliber.com/v1/health',
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
  alertEscalationPolicy: standardEscalation,
  retryStrategy: RetryStrategyBuilder.fixedStrategy({
    baseBackoffSeconds: 30,
    maxRetries: 2,
    maxDurationSeconds: 600,
    sameRegion: true,
  }),
  runParallel: false,
})
