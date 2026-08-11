import { BrowserCheck, Frequency } from 'checkly/constructs';
import { shogunitoGroup } from '../groups';
import * as path from 'path';

new BrowserCheck('shogunito-web-ui-browser', {
  name: 'Shogunito Web UI (Browser)',
  activated: true,
  group: shogunitoGroup,
  frequency: Frequency.EVERY_12H,
  locations: ['eu-central-1'],
  runtimeId: '2025.04',
  code: {
    entrypoint: path.join(__dirname, 'web-ui.spec.ts'),
  },
});
