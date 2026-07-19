import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 30 * 1000,
  expect: {
    timeout: 5000,
  },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 4,
  reporter: [
    ['html', { open: 'never' }],
    ['list']
  ],
  use: {
    baseURL: process.env.GATEWAY_URL || 'https://localhost:8080',
    extraHTTPHeaders: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer local-test-key',
    },
    trace: 'on-first-retry',
    ignoreHTTPSErrors: true,
  },
});
