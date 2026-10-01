import {defineConfig} from '@playwright/test';
export default defineConfig({
  testDir: '.', testMatch: '*.spec.mjs', workers: 1, timeout: 30000,
  use: {baseURL: process.env.THOUGHTS_BROWSER_URL || 'http://127.0.0.1:7777', viewport: {width: 1200, height: 850}},
  projects: [{name: 'chromium', use: {browserName: 'chromium', permissions: ['clipboard-read','clipboard-write']}}, {name: 'firefox', use: {browserName: 'firefox'}}]
});
