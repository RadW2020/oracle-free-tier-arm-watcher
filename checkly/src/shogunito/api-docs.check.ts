import { ApiCheck, Frequency, AssertionBuilder } from 'checkly/constructs';
import { shogunitoGroup } from '../groups';

new ApiCheck('api-docs-check', {
  name: 'Shogunito API Swagger JSON',
  activated: true,
  group: shogunitoGroup,
  frequency: Frequency.EVERY_6H,
  request: {
    url: '{{API_URL}}/api/v1/docs-json',
    method: 'GET',
    assertions: [
      AssertionBuilder.statusCode().equals(200),
      AssertionBuilder.jsonBody('info.title').contains('Shogunito API'),
    ],
  },
});
