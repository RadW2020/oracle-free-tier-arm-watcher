import { ApiCheck, AssertionBuilder, Frequency } from 'checkly/constructs'
import { raulEmailAlert } from '../alert-channels'
import { crossRegionRetry, LOCATIONS } from '../retries'
import { singleAlertEscalation } from '../escalation'

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
 * Umbral en 50.000 paquetes/hora y no en «cualquier descarte»: comprobado el
 * 11/10/2026, con /v1 ya en producción y el techo de 20 Mbps de AIDRA activo
 * (DOWNLOAD_RATE_LIMIT_MBPS=20). Aun así OCI descarta paquetes casi a diario:
 * 105 horas con descartes entre el 26/09 y el 11/10 (~6,5 al día), sobre todo
 * en el ciclo de las 05:15 CEST de AIDRA, con ~20.000/hora. El 10 y el
 * 11/10, en esas horas, ningún check de servicio pasó de 310 ms. Con `< 1`
 * esto habría avisado varias veces al día por algo que no se nota. Sólo 3
 * horas pasaron de 50.000 en esas dos semanas; el 17/09, el que sí tumbó el
 * host, fueron ~81.000 en 6 minutos.
 *
 * Coste: 730 API runs/mes (de ~6.570 a ~7.300 de 10.000).
 *
 * Avisa una vez, sin recordatorios (`singleAlertEscalation`): cuando llega el
 * email la ráfaga ya ha pasado, y el dato no cambia hasta el siguiente run.
 */
new ApiCheck('oci-ingress-throttle-drops', {
  name: 'OCI Ingress Throttle Drops',
  description: 'Causa, no síntoma. Falla si el shaper de OCI ha descartado 50.000 paquetes de entrada o más en la última hora (`$.saturation.ingressThrottleDropsLastHour`). Esos descartes afectan a todos los servicios del host. Si llega junto a avisos de Shogunito, Strong Core o AIDRA, la causa es la red de OCI, no esas apps: ver POSTMORTEM-2026-09-17.md. Lee la lectura guardada en `/v1/status`, sin llamar a OCI.',
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
      AssertionBuilder.jsonBody('$.saturation.ingressThrottleDropsLastHour').lessThan('50000'),
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
  alertEscalationPolicy: singleAlertEscalation,
  retryStrategy: crossRegionRetry,
  runParallel: false,
})
