import { CheckGroupV2, RetryStrategyBuilder } from 'checkly/constructs'
import { raulEmailAlert } from './alert-channels'
import { standardEscalation } from './escalation'

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
  alertEscalationPolicy: standardEscalation,
  retryStrategy: RetryStrategyBuilder.noRetries(),
  runParallel: false,
})

export const aidraGroup = new CheckGroupV2('aidra', {
  name: 'AIDRA',
  locations: [
    'eu-central-1',
  ],
  tags: [
    'aidra',
  ],
  alertChannels: [
    raulEmailAlert,
  ],
  alertEscalationPolicy: standardEscalation,
  retryStrategy: RetryStrategyBuilder.noRetries(),
  runParallel: false,
})

export const strongCoreGroup = new CheckGroupV2('strong-core', {
  name: 'Strong Core',
  locations: [
    'eu-central-1',
  ],
  tags: [
    'strong-core',
  ],
  alertChannels: [
    raulEmailAlert,
  ],
  alertEscalationPolicy: standardEscalation,
  retryStrategy: RetryStrategyBuilder.noRetries(),
  runParallel: false,
})
