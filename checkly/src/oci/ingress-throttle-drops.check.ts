import { ApiCheck, AssertionBuilder, Frequency } from 'checkly/constructs'
import { crossRegionRetry, LOCATIONS } from '../retries'

/**
 * Diagnóstico, no aviso: corre y guarda su historial, pero no manda emails.
 *
 * Nació como acción 3 del POSTMORTEM-2026-09-17.md, para que el email de un
 * servicio caído llegara junto a su causa. Pero medido el 20/10/2026, los
 * descartes no degradan los servicios:
 *
 * - En 7 días, 42 runs de checks de servicio cayeron en minutos con
 *   descartes, hasta 120.000/min, y ninguna fue lenta (mediana: 1,0× su
 *   tiempo normal).
 * - Las 25 runs lentas de esa semana fueron todas en minutos sin descartes.
 *   El 13/10, de 14:13 a 16:20 UTC, con el host en reposo, el handshake TCP
 *   pasó de 42 ms a 250-1.100 ms: pérdida en el camino, fuera de la VNIC.
 * - Los síntomas del 17/09 tampoco caen en minutos con descartes (ver el
 *   addendum del postmortem).
 *
 * Y descartes hay con cada despliegue de Coolify, cada descarga de AIDRA,
 * cada subida grande a Shogunito (19/10, 341 MB en 95 s: 323.000 en dos
 * minutos) y la copia nocturna de downtrace-db. El shaper corta ráfagas, no
 * caudal medio, así que eso no va a dejar de pasar. Con email, el 19/10 este
 * check avisó de una subida propia que no afectó a nada. Un aviso que salta
 * sin efecto medible sólo enseña a ignorar el correo.
 *
 * Sigue activo para cuando un servicio sí falle: su historial dice en un
 * vistazo si hubo descartes a esa hora.
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
 * a las 05:15 CEST, con ~20.000/hora. No es AIDRA, como se creyó: ese ciclo
 * no descarga nada si no hay escena nueva (14/10). Coincide con la copia
 * diaria de downtrace-db a S3 (03:15 UTC), que mete ~176 MB de entrada;
 * probablemente Coolify vuelve a bajar su imagen helper, sin confirmar. El 10 y el
 * 11/10, en esas horas, ningún check de servicio pasó de 310 ms. Con `< 1`
 * esto habría avisado varias veces al día por algo que no se nota. Sólo 3
 * horas pasaron de 50.000 en esas dos semanas; el 17/09, el que sí tumbó el
 * host, fueron ~81.000 en 6 minutos.
 *
 * Coste: 730 API runs/mes (de ~6.570 a ~7.300 de 10.000).
 *
 */
new ApiCheck('oci-ingress-throttle-drops', {
  name: 'OCI Ingress Throttle Drops',
  description: 'Causa, no síntoma. Falla si el shaper de OCI ha descartado 50.000 paquetes de entrada o más en la última hora (`$.saturation.ingressThrottleDropsLastHour`). No avisa: medido el 20/10/2026, los descartes no degradaron ningún servicio. Sirve para descartar o confirmar la red de OCI cuando otro check falle: ver el addendum de POSTMORTEM-2026-09-17.md. Lee la lectura guardada en `/v1/status`, sin llamar a OCI.',
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
  // Sin canales a propósito: ver el comentario de arriba.
  alertChannels: [],
  retryStrategy: crossRegionRetry,
  runParallel: false,
})
