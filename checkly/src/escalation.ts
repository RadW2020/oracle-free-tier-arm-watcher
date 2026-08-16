import { AlertEscalationBuilder } from 'checkly/constructs'

/**
 * Política de escalación de toda la cuenta.
 *
 * Antes vivía duplicada literalmente en 8 checks, con `amount: 0`: un único
 * email por incidencia y ningún recordatorio. Con un solo canal de aviso
 * (email a raul@uliber.com) eso significaba que un correo perdido —spam,
 * visto de madrugada, cerrado sin querer— era la incidencia entera perdida,
 * porque nada volvía a insistir.
 *
 * Ahora: aviso al primer run fallido, y hasta 2 recordatorios cada 10 min
 * mientras siga cayéndose. Tres oportunidades de enterarte en 20 minutos en
 * vez de una sola.
 *
 * Los recordatorios paran solos cuando el check se recupera, y la
 * recuperación también avisa (`sendRecovery: true` en el canal), así que no
 * hay riesgo de quedarse insistiendo sobre algo ya resuelto.
 */
export const standardEscalation = AlertEscalationBuilder.runBasedEscalation(
  1,
  {
    amount: 2,
    interval: 10,
  },
  {
    enabled: false,
    percentage: 10,
  },
)
