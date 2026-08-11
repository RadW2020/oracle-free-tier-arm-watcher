import { defineConfig } from 'checkly';

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
