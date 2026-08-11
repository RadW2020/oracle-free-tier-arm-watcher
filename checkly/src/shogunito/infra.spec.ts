import { test, expect } from '@playwright/test';
import { getAPIResponseTime, markCheckAsDegraded } from '@checkly/playwright-helpers';

const API_URL = process.env.API_URL || 'https://api.your-domain.com';
const WEB_URL = process.env.WEB_URL || 'https://your-domain.com';

const RESPONSE_TIME_THRESHOLD = 800; // ms

test('Shogunito Infrastructure Health', async ({ request }) => {
  // ── Step 1: API Core Health ────────────────────────────────────────
  await test.step('API core health', async () => {
    const response = await request.get(`${API_URL}/health`);

    expect
      .soft(getAPIResponseTime(response), 'API health response too slow')
      .toBeLessThanOrEqual(RESPONSE_TIME_THRESHOLD);

    expect(response).toBeOK();

    const body = await response.json();
    expect(body.success).toBe(true);
    expect(body.data.status).toBe('ok');
  });

  // ── Step 2: Auth Service Health ────────────────────────────────────
  await test.step('Auth service health', async () => {
    const response = await request.get(`${API_URL}/api/v1/auth/health`);

    expect
      .soft(getAPIResponseTime(response), 'Auth service response too slow')
      .toBeLessThanOrEqual(RESPONSE_TIME_THRESHOLD);

    expect(response).toBeOK();

    const body = await response.json();
    expect(body.success).toBe(true);
    expect(body.data.status).toBe('ok');
    expect(body.data.service).toBe('auth');
  });

  // ── Step 3: Swagger docs alive and valid ───────────────────────────
  await test.step('API docs (Swagger JSON)', async () => {
    const response = await request.get(`${API_URL}/api/v1/docs-json`);

    expect
      .soft(getAPIResponseTime(response), 'Swagger response too slow')
      .toBeLessThanOrEqual(RESPONSE_TIME_THRESHOLD);

    expect(response).toBeOK();

    const docs = await response.json();
    expect(docs.info.title).toContain('Shogunito');
    expect(docs.paths).toBeTruthy();

    // Verify critical endpoints exist in the OpenAPI spec
    expect(docs.paths['/api/v1/auth/login']).toBeTruthy();
    expect(docs.paths['/api/v1/projects']).toBeTruthy();
    expect(docs.paths['/api/v1/episodes']).toBeTruthy();
  });

  // ── Step 4: Web UI responds ────────────────────────────────────────
  await test.step('Web UI is up', async () => {
    const response = await request.get(WEB_URL);

    expect
      .soft(getAPIResponseTime(response), 'Web UI response too slow')
      .toBeLessThanOrEqual(RESPONSE_TIME_THRESHOLD);

    expect(response).toBeOK();
  });

  // ── Step 5: MinIO Object Storage ───────────────────────────────────
  await test.step('MinIO S3 storage health', async () => {
    const response = await request.get(
      `${process.env.MINIO_URL || 'https://minio.your-domain.com'}/minio/health/live`,
    );

    expect
      .soft(getAPIResponseTime(response), 'MinIO response too slow')
      .toBeLessThanOrEqual(RESPONSE_TIME_THRESHOLD);

    expect(response).toBeOK();
  });

  // ── Degraded State ─────────────────────────────────────────────────
  // If all hard assertions passed but some soft assertions failed,
  // the check is marked as "degraded" (yellow) instead of "failed" (red).
  // This means: "everything works, but performance is not ideal."
  if (test.info().errors.length) {
    markCheckAsDegraded('Infrastructure is up but some services responded slowly.');
  }
});
