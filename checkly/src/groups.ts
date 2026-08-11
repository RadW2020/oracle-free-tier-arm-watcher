import { CheckGroupV2, RetryStrategyBuilder } from 'checkly/constructs'
import { raulEmailAlert } from './alert-channels'

export const shogunitoGroup = new CheckGroupV2('shogunito-VwUfFB4z', {
  name: 'Shogunito',
  locations: [
    'eu-central-1',
  ],
  tags: [
    'shogunito',
  ],
  alertChannels: [
    raulEmailAlert,
  ],
  alertEscalationPolicy: 'global',
  retryStrategy: RetryStrategyBuilder.noRetries(),
  runParallel: false,
})

export const eduSchedulerGroup = new CheckGroupV2('edu-scheduler-In2dyugM', {
  name: 'EduScheduler',
  locations: [
    'eu-central-1',
  ],
  tags: [
    'eduscheduler',
  ],
  alertChannels: [
    raulEmailAlert,
  ],
  alertEscalationPolicy: 'global',
  retryStrategy: RetryStrategyBuilder.noRetries(),
  runParallel: false,
})
