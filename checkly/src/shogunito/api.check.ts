import { ApiCheck, Frequency, AssertionBuilder } from 'checkly/constructs';
import { shogunitoGroup } from '../groups';

new ApiCheck('api-health-check', {
  name: 'Shogunito API Health',
  activated: true,
  group: shogunitoGroup,
  frequency: Frequency.EVERY_12H,
  locations: ['eu-central-1', 'us-east-1'],
  request: {
    url: '{{API_URL}}/health',
    method: 'GET',
    assertions: [AssertionBuilder.statusCode().equals(200)],
  },
});
