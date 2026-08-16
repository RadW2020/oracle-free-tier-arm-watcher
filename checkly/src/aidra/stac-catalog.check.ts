import { ApiCheck, AssertionBuilder, Frequency } from 'checkly/constructs'
import { aidraGroup } from '../groups'

/**
 * Catálogo STAC de AIDRA.
 *
 * Es la superficie que consumen clientes externos (QGIS, pystac-client,
 * EODAG), así que su contrato es lo que hay que vigilar, no sólo que
 * responda: `type: Catalog` y `stac_version` son lo que esos clientes
 * exigen para no romperse.
 *
 * Complementa al health: éste sale por la ruta que lee de verdad la BD,
 * así que detecta una degradación que `/api/health` podría no ver.
 */
new ApiCheck('aidra-stac-catalog', {
  name: 'AIDRA STAC Catalog',
  request: {
    url: 'https://aidra-api.uliber.com/api/stac/catalog.json',
    method: 'GET',
    ipFamily: 'IPv4',
    assertions: [
      AssertionBuilder.statusCode().equals(200),
      AssertionBuilder.jsonBody('$.type').equals('Catalog'),
      AssertionBuilder.jsonBody('$.stac_version').equals('1.0.0'),
      AssertionBuilder.jsonBody('$.id').equals('aidra'),
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
