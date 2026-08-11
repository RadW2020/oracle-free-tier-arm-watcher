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
        // Triple llave a propósito: Checkly templatiza con Handlebars y {{x}}
        // escapa HTML, así que el '=' del padding base64 del token salía como
        // '&#x3D;' y el endpoint devolvía 401. {{{x}}} no escapa.
        // Tampoco lleva `locked`: una cabecera bloqueada se guarda como secreto
        // opaco y no se interpola. El secreto vive en la variable de cuenta
        // CIAOBOX_CRON_TOKEN, que sí está locked; aquí solo hay una referencia.
        value: 'Bearer {{{CIAOBOX_CRON_TOKEN}}}',
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
