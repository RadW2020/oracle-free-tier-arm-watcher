import { ApiCheck, Frequency, AssertionBuilder } from 'checkly/constructs';
import { shogunitoGroup } from '../groups';

new ApiCheck('minio-health-check', {
  name: 'Shogunito MinIO S3 API',
  activated: true,
  group: shogunitoGroup,
  frequency: Frequency.EVERY_12H,
  locations: ['eu-central-1', 'us-east-1'],
  request: {
    url: '{{MINIO_URL}}/minio/health/live',
    method: 'GET',
    assertions: [AssertionBuilder.statusCode().equals(200)],
  },
});
