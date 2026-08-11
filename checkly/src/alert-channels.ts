import { EmailAlertChannel } from 'checkly/constructs'

export const raulEmailAlert = new EmailAlertChannel('email-BiJtwdfW', {
  address: 'raul@uliber.com',
  sslExpiry: true,
})
