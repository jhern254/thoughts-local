import {defineConfig} from '@playwright/test';
export default defineConfig({
  testDir: '.', testMatch: '*.spec.mjs', workers: 1, timeout: 30000,
  use: {baseURL: process.env.THOUGHTS_BROWSER_URL || 'http://127.0.0.1:7777', viewport: {width: 1200, height: 850}},
  // A known sRGB display makes headless Firefox color conversion deterministic.
  // Color-management mode 2 manages tagged images; enablev4 accepts the
  // synthetic v4 ICC fixture rather than silently ignoring its color transform.
  // Firefox PCM acceptance uses its synthetic device, never a physical mic.
  projects: [{name: 'chromium', use: {browserName: 'chromium', permissions: ['clipboard-read','clipboard-write']}}, {name: 'firefox', use: {browserName: 'firefox', launchOptions: {firefoxUserPrefs: {'gfx.color_management.force_srgb': true, 'gfx.color_management.mode': 2, 'gfx.color_management.enablev4': true, 'media.navigator.streams.fake': true, 'media.navigator.permission.disabled': true}}}}]
});
