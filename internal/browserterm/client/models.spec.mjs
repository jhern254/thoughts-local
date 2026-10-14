import {test, expect} from '@playwright/test';
import {screen, clickControl} from './visual-helpers.mjs';
import {fakeMicrophone} from './capture-fixture.mjs';

async function activateControl(page, label) {
  await expect(screen(page)).toContainText(label);
  await clickControl(page, label);
}

test.skip(process.env.THOUGHTS_MODELS_ONBOARDING_TEST !== '1', 'requires the injected model setup fixture');

test('model onboarding preserves writing and enables dictation after cancel and retry', async ({page}, testInfo) => {
  await fakeMicrophone(page, 0.25, testInfo.project.name === 'firefox');
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Enable local speech-to-text');
  await expect(screen(page)).toContainText('Events');
  if (process.env.THOUGHTS_BROWSER_ARTIFACTS) await page.screenshot({path: `${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-models-onboarding.png`});
  await page.setViewportSize({width: 400, height: 600});
  await expect(screen(page)).toContainText('Download and enable');
  await expect(screen(page)).toContainText('Not now');
  if (process.env.THOUGHTS_BROWSER_ARTIFACTS) await page.screenshot({path: `${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-models-narrow.png`});
  await page.keyboard.press('Enter');
  await expect(screen(page)).not.toContainText('Enable local speech-to-text');
  await page.reload();
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Enable local speech-to-text');
  await page.keyboard.press('Enter');
  await page.setViewportSize({width: 1200, height: 850});
  await page.keyboard.press('F8');
  await expect(screen(page)).toContainText('Create thought');
  await page.keyboard.type('onboarding draft');
  await page.keyboard.press('Control+,');
  await expect(screen(page)).toContainText('Visual');
  await activateControl(page, 'Models');
  await activateControl(page, 'Download and enable');
  await expect(screen(page)).toContainText('71.2 / 142.3 MB');
  if (process.env.THOUGHTS_BROWSER_ARTIFACTS) await page.screenshot({path: `${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-models-progress.png`});
  await activateControl(page, 'Keep writing');
  await expect(screen(page)).toContainText('onboarding draft');
  await page.keyboard.type(' while downloading');
  await page.keyboard.press('Control+,');
  await expect(screen(page)).toContainText('Visual');
  await activateControl(page, 'Models');
  await expect(screen(page)).toContainText('71.2 / 142.3 MB');
  await activateControl(page, 'Cancel download');
  await expect(screen(page)).toContainText('Download cancelled');
  await activateControl(page, 'Retry download');
  await expect(screen(page)).toContainText('Speech is ready');
  await activateControl(page, 'Done');
  await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('onboarding draft while downloading');
  await page.keyboard.press('F8');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.evaluate(() => window.voiceProbe.grant(0));
  await expect(page.locator('#status')).toHaveText('Transcribing…');
  await expect(screen(page)).toContainText('synthetic speech');
  await page.keyboard.press('F8');
  await expect(screen(page)).toContainText('F8: Record');
});
