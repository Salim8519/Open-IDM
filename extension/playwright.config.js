import { defineConfig } from '@playwright/test';

// End-to-end tests load the unpacked extension into Playwright's Chromium (Chrome for
// Testing) and talk to a real native messaging host + fake app. See tests/e2e/.
export default defineConfig({
  testDir: 'tests/e2e',
  timeout: 120_000,
  expect: { timeout: 15_000 },
  workers: 1,
  fullyParallel: false,
  reporter: [['list']],
  retries: 0,
});
