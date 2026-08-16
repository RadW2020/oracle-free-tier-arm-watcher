import { Dashboard } from 'checkly/constructs'

/**
 * Dashboard general de la cuenta (el plan incluye 1).
 *
 * Sustituye a `shogun-status`, que ocupaba el único slot desde la época en
 * que la config vivía en el repo de Shogunito y que filtraba por los tags
 * `critical` y `cloudflare` — tags que ningún check tiene, así que llevaba
 * tiempo mostrando cero checks. Se borró para liberar el cupo y que éste
 * naciera ya como código.
 *
 * Sin `tags` no filtra nada: entran los 17 checks de las cinco áreas
 * (Shogunito, Ciaobox, EduScheduler, AIDRA e infraestructura OCI), que es
 * justamente la vista única que no existía.
 *
 * No sustituye a la status page: aquélla es la cara pública por servicio
 * (operativo / degradado); ésta es la parrilla de checks con sus métricas.
 */
new Dashboard('uliber-overview', {
  header: 'Uliber & Co — Monitoring',
  description: 'Estado y latencias de todos los servicios: Shogunito, Ciaobox, EduScheduler, AIDRA e infraestructura OCI.',
  customUrl: 'uliber-status',
  width: 'FULL',
  refreshRate: 60,
  paginate: false,
  checksPerPage: 20,
  hideTags: false,
  expandChecks: false,
  showHeader: true,
  showP95: true,
  showP99: true,
})
