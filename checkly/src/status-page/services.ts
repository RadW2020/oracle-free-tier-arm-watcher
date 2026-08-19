import { StatusPageService } from 'checkly/constructs'

export const aidraApiService = new StatusPageService('aidra-api', {
  name: 'AIDRA API',
})

export const aidraStacService = new StatusPageService('aidra-stac', {
  name: 'AIDRA STAC Catalog',
})

export const aidraDashboardService = new StatusPageService('aidra-dashboard', {
  name: 'AIDRA Dashboard (Grafana)',
})

export const ciaoboxCronService = new StatusPageService('ciaobox-cron-57Lpj5pI', {
  name: 'Ciaobox Cron',
})

export const ciaoboxWebService = new StatusPageService('ciaobox-web-pTQ8OCOR', {
  name: 'Ciaobox Web',
})

export const oracleFreeTierMonitorService = new StatusPageService('oracle-free-tier-monitor-JN75Xj5w', {
  name: 'Oracle Free Tier Monitor',
})

export const shogunitoApiDocsService = new StatusPageService('shogunito-api-docs-i6kMn1Yd', {
  name: 'Shogunito API Docs',
})

export const shogunitoApiService = new StatusPageService('shogunito-api-3xvXYSLm', {
  name: 'Shogunito API',
})

export const shogunitoInfrastructureService = new StatusPageService('shogunito-infrastructure-qTiKe2gK', {
  name: 'Shogunito Infrastructure',
})

export const shogunitoMinIoS3Service = new StatusPageService('shogunito-min-io-s-3-EsaMeo2V', {
  name: 'Shogunito MinIO S3',
})

export const shogunitoWebBrowserService = new StatusPageService('shogunito-web-browser-1lmWW3o9', {
  name: 'Shogunito Web (Browser)',
})

export const shogunitoWebUiService = new StatusPageService('shogunito-web-ui-jsSv3NTY', {
  name: 'Shogunito Web UI',
})

/**
 * Los dos servicios de Strong Core no son uno por check.
 *
 * La app tiene 3 checks, pero `Strong Core Historial (SSR)` no es un
 * servicio que nadie consuma por separado: es un sondeo más profundo de la
 * misma web, el que distingue "sirve el prerender cacheado" de "renderiza
 * bajo demanda". Aquí cuenta como parte de `Strong Core Web`.
 *
 * `Strong Core Login` sí es aparte: con acceso sólo por magic link, "no
 * puedo entrar" es una caída distinta de "la web no carga", y es lo que
 * vigila el check de `/api/auth/providers`.
 *
 * Mismo criterio que Ciaobox, que tiene 2 servicios para 3 checks.
 */
export const strongCoreWebService = new StatusPageService('strong-core-web-service', {
  name: 'Strong Core Web',
})

export const strongCoreLoginService = new StatusPageService('strong-core-login-service', {
  name: 'Strong Core Login',
})

