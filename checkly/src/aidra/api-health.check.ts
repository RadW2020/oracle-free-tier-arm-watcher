import { ApiCheck, AssertionBuilder, Frequency } from 'checkly/constructs'
import { aidraGroup } from '../groups'

/**
 * Health del API de AIDRA.
 *
 * `/api/health` no es un 200 vacío: devuelve el estado de los componentes
 * (Postgres/PostGIS, modelos cargados, scheduler). Por eso esto es un
 * ApiCheck y no un UrlMonitor — las assertions sobre el JSON son lo que
 * distingue "el proceso responde" de "el pipeline puede trabajar".
 *
 * `db: connected` es la que de verdad importa: sin PostGIS no hay
 * detecciones ni STAC, aunque el proceso siga vivo y devolviendo 200.
 */
new ApiCheck('aidra-api-health', {
  name: 'AIDRA API Health',
  request: {
    url: 'https://aidra-api.uliber.com/api/health',
    method: 'GET',
    ipFamily: 'IPv4',
    assertions: [
      AssertionBuilder.statusCode().equals(200),
      AssertionBuilder.jsonBody('$.status').equals('ok'),
      AssertionBuilder.jsonBody('$.db').equals('connected'),
      AssertionBuilder.jsonBody('$.scheduler').equals('running'),
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
    'aidra',
  ],
  frequency: Frequency.EVERY_6H,
  group: aidraGroup,
  runParallel: false,
})
