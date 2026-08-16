import { EmailAlertChannel } from 'checkly/constructs'

/**
 * Único canal de alerta de la cuenta.
 *
 * `sendDegraded: true` importa más de lo que parece: todos los checks
 * declaran `degradedResponseTime`, pero con el canal en `false` ese umbral
 * no producía ningún aviso — un servicio lento pero vivo pasaba
 * desapercibido hasta que se caía del todo.
 */
export const raulEmailAlert = new EmailAlertChannel('email-BiJtwdfW', {
  address: 'raul@uliber.com',
  sendFailure: true,
  sendRecovery: true,
  sendDegraded: true,
  sslExpiry: true,
  sslExpiryThreshold: 30,
})
