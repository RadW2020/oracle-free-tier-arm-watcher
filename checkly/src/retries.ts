import type { Region } from 'checkly'
import { RetryStrategyBuilder } from 'checkly/constructs'

/**
 * Localizaciones desde las que se vigila todo.
 *
 * Dos, y las dos europeas. Frankfurt era la única hasta el 18/09/2026, y eso
 * dejaba un punto ciego que costó una mañana entera de correos: cuando el
 * camino entre los runners de Frankfurt y la IP de Madrid empezó a tragarse
 * paquetes, no había forma de distinguir "el servidor está caído" de "no se
 * llega al servidor desde ahí". El host estaba impecable —cero descartes en
 * el kernel, conntrack al 0,4 %, la cola de SYN sin desbordar ni una vez en
 * 50 días— y aun así los checks daban timeout de 20 s.
 *
 * Londres y no us-east-1 (que es lo que declaraban a mano algunos checks de
 * Shogunito, sin efecto porque el grupo pisa las localizaciones del check):
 * el servicio vive en Madrid, y cruzar el Atlántico para vigilarlo añade
 * ~100 ms a cada medición y falsearía los umbrales de degradación. París
 * sería mejor por cercanía, pero el plan actual sólo permite eu-central-1,
 * eu-west-2, us-east-1, us-west-1 y las dos de Asia-Pacífico.
 *
 * Con `runParallel: false` esto NO duplica el consumo: Checkly rota entre
 * localizaciones, una por ejecución.
 */
export const LOCATIONS: (keyof Region)[] = ['eu-central-1', 'eu-west-2']

/**
 * Reintento por defecto de toda la cuenta.
 *
 * La clave es `sameRegion: false`: si un intento falla, el siguiente sale de
 * la OTRA localización. Un SYN perdido en el camino a Frankfurt deja de ser
 * un email y pasa a ser lo que es —ruido de red—, mientras que una caída de
 * verdad falla desde las dos y sí avisa.
 *
 * Antes los tres grupos iban con `noRetries()`: un único paquete perdido
 * bastaba para despertar a alguien. El 18/09/2026 eso fueron siete avisos en
 * una mañana, ninguno con un servicio realmente caído detrás.
 *
 * 2 reintentos de 30 s y tope de 600 s: cabe de sobra antes del siguiente
 * run incluso en los checks de 5 minutos.
 */
export const crossRegionRetry = RetryStrategyBuilder.fixedStrategy({
  baseBackoffSeconds: 30,
  maxRetries: 2,
  maxDurationSeconds: 600,
  sameRegion: false,
})

/**
 * Reintento para los uptime monitors.
 *
 * Mismo criterio —el reintento sale de la otra localización— pero con un
 * único intento: el plan actual no admite estrategias de varios reintentos
 * en `UrlMonitor`, y como el grupo pisa la estrategia de sus checks, un
 * grupo que contenga monitores tiene que usar ésta o el despliegue se cae
 * entero con "The unlimited retry strategy isn't supported on your plan".
 */
export const monitorRetry = RetryStrategyBuilder.singleRetry({
  baseBackoffSeconds: 30,
  sameRegion: false,
})
