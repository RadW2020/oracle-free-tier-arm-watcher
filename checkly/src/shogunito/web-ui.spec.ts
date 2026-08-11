import { test, expect } from '@playwright/test';

const WEB_URL = process.env.WEB_URL || 'https://your-domain.com';

test('Shogunito Web UI renders and login page loads', async ({ page }) => {
  // -- Step 1: Landing redirects to login (protected route)
  const response = await page.goto(WEB_URL);
  expect(response?.status()).toBeLessThan(400);

  await page.waitForURL('**/login');

  // -- Step 2: Login form is interactive
  const form = page.locator('[data-testid="login-form"]');
  await expect(form).toBeVisible();

  const emailInput = page.locator('input#email');
  const passwordInput = page.locator('input#password');
  const submitButton = page.locator('[data-testid="login-submit-button"]');

  await expect(emailInput).toBeVisible();
  await expect(passwordInput).toBeVisible();
  await expect(submitButton).toBeVisible();
  await expect(submitButton).toHaveText('Sign in');

  // -- Step 3: Page title and branding
  await expect(page.locator('h2')).toContainText('Shogunito');

  // -- Step 4: Navigation links exist
  await expect(page.locator('a[href="/register"]')).toBeVisible();
  await expect(page.locator('a[href="/forgot-password"]')).toBeVisible();

  // -- Step 5: Screenshot for visual reference
  await page.screenshot({ path: 'login-page.png', fullPage: true });
});
