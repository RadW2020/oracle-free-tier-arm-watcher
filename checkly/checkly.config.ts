import { defineConfig } from 'checkly';
import { raulEmailAlert } from './src/alert-channels';
import { standardEscalation } from './src/escalation';

/**
 * Configuration as code for the whole Checkly account (Uliber & Co).
 *
 * ⚠️ `logicalId` is the project's identity in Checkly. It stays as
 * 'shogunito-project' on purpose: the six Shogunito checks already live in
 * Checkly under that id. Renaming it would make the CLI treat them as new
 * resources — it would create duplicates and orphan the originals, losing
 * their run history and metrics. Only `projectName` is cosmetic.
 *
 * See ./README.md for the layout and the deploy workflow.
 */
export default defineConfig({
  projectName: 'Uliber Monitoring',
  logicalId: 'shogunito-project',
  repoUrl: 'https://github.com/RadW2020/oracle-free-tier-arm-watcher',
  checks: {
    checkMatch: 'src/**/*.check.ts',
    ignoreDirectoriesMatch: [],
    /**
     * Canal por defecto para TODO check del proyecto.
     *
     * Es la red de seguridad contra el fallo más silencioso posible: un
     * check nuevo que no esté en un grupo suscrito ni declare su propio
     * `alertChannels` no avisaría a nadie, y no hay forma de notarlo salvo
     * que algo se caiga y nadie se entere. Con este default, un check
     * nuevo avisa por omisión y hay que desactivarlo a propósito.
     *
     * Los checks que ya declaran `alertChannels` o heredan de su grupo no
     * cambian: esto sólo rellena el hueco cuando no hay nada.
     */
    alertChannels: [raulEmailAlert],
    /**
     * Misma red de seguridad para la escalación: un check que no declare la
     * suya ni herede la de un grupo insistirá igualmente con recordatorios,
     * en vez de caer en el `amount: 0` que dejaba las incidencias en un
     * único email sin repetición.
     */
    alertEscalationPolicy: standardEscalation,
    /**
     * Only *.browser.spec.ts files are auto-converted into browser checks.
     * The Playwright specs under src/shogunito are referenced explicitly by
     * their BrowserCheck / MultiStepCheck constructs, so they must not match
     * this pattern or they would be deployed twice.
     */
    browserChecks: {
      testMatch: 'src/**/*.browser.spec.ts',
    },
  },
  cli: {
    runLocation: 'eu-central-1',
  },
});
