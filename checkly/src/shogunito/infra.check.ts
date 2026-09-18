import { MultiStepCheck, Frequency } from 'checkly/constructs';
import { shogunitoGroup } from '../groups';
import * as path from 'path';

new MultiStepCheck('shogunito-infra-check', {
  name: 'Shogunito Infrastructure (MultiStep)',
  activated: true,
  // Las localizaciones y la estrategia de reintento las fija el grupo
  // (src/groups.ts -> src/retries.ts): declararlas aquí no tiene efecto.
  group: shogunitoGroup,
  frequency: Frequency.EVERY_3H,
  runtimeId: '2025.04',
  code: {
    entrypoint: path.join(__dirname, 'infra.spec.ts'),
  },
});
