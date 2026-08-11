import { MultiStepCheck, Frequency } from 'checkly/constructs';
import { shogunitoGroup } from '../groups';
import * as path from 'path';

new MultiStepCheck('shogunito-infra-check', {
  name: 'Shogunito Infrastructure (MultiStep)',
  activated: true,
  group: shogunitoGroup,
  frequency: Frequency.EVERY_6H,
  locations: ['eu-central-1', 'us-east-1'],
  runtimeId: '2025.04',
  code: {
    entrypoint: path.join(__dirname, 'infra.spec.ts'),
  },
  retryStrategy: {
    type: 'LINEAR',
    maxRetries: 2,
    baseBackoffSeconds: 30,
  },
});
