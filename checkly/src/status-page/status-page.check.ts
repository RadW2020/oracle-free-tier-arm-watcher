import { StatusPage } from 'checkly/constructs'
import {
  aidraApiService,
  aidraStacService,
  aidraDashboardService,
  oracleFreeTierMonitorService,
  shogunitoApiService,
  shogunitoMinIoS3Service,
  shogunitoWebUiService,
  shogunitoApiDocsService,
  shogunitoInfrastructureService,
  shogunitoWebBrowserService,
  ciaoboxCronService,
  ciaoboxWebService,
  strongCoreWebService,
  strongCoreLoginService,
} from './services'

new StatusPage('uliber-co-system-status-BDS7AS4c', {
  name: 'Uliber & Co - System Status',
  url: 'uliber-status',
  cards: [
    {
      name: 'OCI Infrastructure',
      services: [
        oracleFreeTierMonitorService,
      ],
    },
    {
      name: 'Shogunito',
      services: [
        shogunitoApiService,
        shogunitoMinIoS3Service,
        shogunitoWebUiService,
        shogunitoApiDocsService,
        shogunitoInfrastructureService,
        shogunitoWebBrowserService,
      ],
    },
    {
      name: 'Ciaobox',
      services: [
        ciaoboxCronService,
        ciaoboxWebService,
      ],
    },
    {
      name: 'AIDRA',
      services: [
        aidraApiService,
        aidraStacService,
        aidraDashboardService,
      ],
    },
    {
      name: 'Strong Core',
      services: [
        strongCoreWebService,
        strongCoreLoginService,
      ],
    },
  ],
  defaultTheme: 'AUTO',
})
