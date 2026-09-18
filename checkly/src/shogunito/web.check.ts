import { ApiCheck, Frequency, AssertionBuilder } from 'checkly/constructs';
import { shogunitoGroup } from '../groups';

new ApiCheck('web-health-check', {
  name: 'Shogunito Web UI',
  activated: true,
  group: shogunitoGroup,
  frequency: Frequency.EVERY_1H,
  request: {
    url: '{{WEB_URL}}',
    method: 'GET',
    assertions: [AssertionBuilder.statusCode().equals(200)],
  },
});
