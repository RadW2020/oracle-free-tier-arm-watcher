import { CheckGroupV2 } from 'checkly/constructs'
import { raulEmailAlert } from './alert-channels'
import { standardEscalation } from './escalation'
import { crossRegionRetry, LOCATIONS, monitorRetry } from './retries'

export const shogunitoGroup = new CheckGroupV2('shogunito-VwUfFB4z', {
  name: 'Shogunito',
  locations: LOCATIONS,
  tags: [
    'shogunito',
  ],
  alertChannels: [
    raulEmailAlert,
  ],
  alertEscalationPolicy: standardEscalation,
  retryStrategy: crossRegionRetry,
  runParallel: false,
})

export const aidraGroup = new CheckGroupV2('aidra', {
  name: 'AIDRA',
  locations: LOCATIONS,
  tags: [
    'aidra',
  ],
  alertChannels: [
    raulEmailAlert,
  ],
  alertEscalationPolicy: standardEscalation,
  // Este grupo contiene uptime monitors y el grupo pisa la estrategia
  // de sus checks: con varios reintentos el despliegue no pasa.
  retryStrategy: monitorRetry,
  runParallel: false,
})

export const strongCoreGroup = new CheckGroupV2('strong-core', {
  name: 'Strong Core',
  locations: LOCATIONS,
  tags: [
    'strong-core',
  ],
  alertChannels: [
    raulEmailAlert,
  ],
  alertEscalationPolicy: standardEscalation,
  // Este grupo contiene uptime monitors y el grupo pisa la estrategia
  // de sus checks: con varios reintentos el despliegue no pasa.
  retryStrategy: monitorRetry,
  runParallel: false,
})
