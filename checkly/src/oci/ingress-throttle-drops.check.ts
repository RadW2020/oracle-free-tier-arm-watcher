import { ApiCheck, AssertionBuilder, Frequency } from 'checkly/constructs'
import { raulEmailAlert } from '../alert-channels'
import { crossRegionRetry, LOCATIONS } from '../retries'
import { standardEscalation } from '../escalation'

/**
 * Que el próximo email diga la causa, no el síntoma (acción 3 del
 * POSTMORTEM-2026-09-17.md).
 *
 * El 17/09 los checks de Shogunito y Strong Core fallaron con `i/o timeout`
 * mientras la causa —el shaper de OCI tirando paquetes de entrada por una
 * descarga de AIDRA— no aparecía en ningún aviso. Este check falla cuando OCI
 * ha descartado paquetes en la última hora, así que llega junto a los otros y
 * dice por qué.
 *
 * Lee /v1/status y no /usage a propósito: /v1 sirve la lectura guardada del
 * watcher (se refresca cada 15 min) y no hace ninguna llamada a OCI, mientras
 * que /usage hace 13 + N en cada petición. Por eso puede ir cada hora sin
 * gastar la API que vigila la factura, cosa que el Free Tier Monitor no puede
 * (va a 3 h por eso).
 *
 * Cobertura sin huecos: cada lectura cubre la hora anterior y se toma como
 * mucho 15 min antes del run, así que con runs cada hora las ventanas se
 * solapan.
 *
 * ⚠️ Depende de que el watcher con /v1 esté desplegado: desplegar este check
 * antes daría 404. Y mientras el techo de 20 Mbps de AIDRA no esté en
 * producción, avisará con cada descarga de escena: ése es justo el aviso que
 * faltaba, pero hay que contar con él.
 *
 * Coste: 730 API runs/mes (de ~6.570 a ~7.300 de 10.000).
 */
new ApiCheck('oci-ingress-throttle-drops', {
  name: 'OCI Ingress Throttle Drops',
  request: {
    url: '{{ORACLE_MONITOR_URL}}/v1/status',
    method: 'GET',
    ipFamily: 'IPv4',
    headers: [
      {
        key: 'X-API-Key',
        value: '{{ORACLE_MONITOR_API_KEY}}',
      },
    ],
    assertions: [
      AssertionBuilder.statusCode().equals(200),
      // Si la señal no se pudo leer, el 0 de abajo no significaría nada.
      AssertionBuilder.jsonBody('$.saturation.available').equals('true'),
      AssertionBuilder.jsonBody('$.saturation.ingressThrottleDropsLastHour').lessThan('1'),
    ],
  },
  degradedResponseTime: 5000,
  maxResponseTime: 20000,
  activated: true,
  muted: false,
  shouldFail: false,
  locations: LOCATIONS,
  tags: [
    'oci',
    'saturation',
  ],
  frequency: Frequency.EVERY_1H,
  alertChannels: [
    raulEmailAlert,
  ],
  alertEscalationPolicy: standardEscalation,
  retryStrategy: crossRegionRetry,
  runParallel: false,
})
