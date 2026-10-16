import {test, expect} from '@playwright/test';
import {screen, clickControl, chooseImage} from './visual-helpers.mjs';
import {readFileSync} from 'node:fs';
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
  await expect(screen(page)).toContainText('Not now (recommended to download)');
  if (process.env.THOUGHTS_BROWSER_ARTIFACTS) await page.screenshot({path: `${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-models-onboarding.png`});
  await page.setViewportSize({width: 400, height: 600});
  await expect(screen(page)).toContainText('Download and enable');
  await expect(screen(page)).toContainText('Not now');
  if (process.env.THOUGHTS_BROWSER_ARTIFACTS) await page.screenshot({path: `${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-models-narrow.png`});
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await expect(screen(page)).not.toContainText('Enable local speech-to-text');
  await page.reload();
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Enable local speech-to-text');
  await page.keyboard.press('Escape');
  await page.setViewportSize({width: 1200, height: 850});
  await page.keyboard.press('t');
  await expect(screen(page)).toContainText('Create thought');
  await page.keyboard.type('onboarding draft');
  await page.keyboard.press('Control+,');
  await expect(screen(page)).toContainText('Visual');
  await activateControl(page, 'Models');
  await expect(screen(page)).toContainText('Download and enable');
  await page.keyboard.press('Enter');
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

test('model onboarding keeps Visual video controls available through the Options menu', async ({page, request, baseURL}) => {
  const settings = await request.put('/visual/settings', {
    headers: {Origin: new URL(baseURL).origin},
    data: {darkness: 70, framing: {fit: 'fill', zoom: 100, positionX: 5000, positionY: 5000}, speedPercent: 100, userPaused: false},
  });
  expect(settings.ok()).toBe(true);
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Enable local speech-to-text');
  await page.keyboard.press('Escape');
  await page.keyboard.press('Control+,');
  await activateControl(page, 'Visual');
  await expect(screen(page)).toContainText('Background darkness:');
  await expect(screen(page)).not.toContainText('Loading visual');
  await chooseImage(page, {
    name: 'synthetic-video.mp4', mimeType: 'video/mp4',
    buffer: readFileSync(new URL('./fixtures/visual-video.mp4', import.meta.url)),
  });
  await expect(screen(page)).toContainText('Playback speed: 100%');
  await activateControl(page, 'Playback speed: 100%');
  await page.keyboard.press('ArrowLeft');
  await expect(screen(page)).toContainText('Playback speed: 95%');
  await activateControl(page, 'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await page.keyboard.press('Control+,');
  await activateControl(page, 'Visual');
  await expect(screen(page)).toContainText('Playback speed: 95%');
  await activateControl(page, 'Remove background');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await page.keyboard.press('Control+,');
  await activateControl(page, 'Models');
  await expect(screen(page)).toContainText('Download and enable');
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
});
