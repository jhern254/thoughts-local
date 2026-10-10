import {test, expect} from '@playwright/test';
import {fakeMicrophone} from './capture-fixture.mjs';
test.skip(process.env.THOUGHTS_BROWSER_AUDIO_TEST !== '1', 'requires the injected Go fake-consumer host');
const screen = page => page.locator('.xterm-rows');

async function prepareAudioDraft(page) {
  await fakeMicrophone(page, 0.25, test.info().project.name === 'firefox');
  await page.addInitScript(() => {
    window.audioProbe = {audioFrames: 0, terminalAudioFrames: 0, audioConnections: 0, capabilityInURL: false};
    const NativeWebSocket = window.WebSocket;
    window.WebSocket = class extends NativeWebSocket {
      constructor(address, ...options) {
        super(address, ...options);
        this.audioPath = new URL(address).pathname === '/voice/audio';
        if (this.audioPath) {
          window.audioProbe.audioConnections++;
          window.audioProbe.capabilityInURL ||= new URL(address).search !== '';
        }
      }
      send(frame) {
        if (frame instanceof Uint8Array && frame.byteLength === 3200) {
          if (this.audioPath) window.audioProbe.audioFrames++;
          else window.audioProbe.terminalAudioFrames++;
        }
        super.send(frame);
      }
    };
  });
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await page.keyboard.press('t');
  await expect(screen(page)).toContainText('Create thought');
  await page.keyboard.type('typed draft');
}
async function startCapture(page, permissionIndex) {
  await page.keyboard.press('F8');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(permissionIndex + 1);
  await page.evaluate(index => window.voiceProbe.grant(index), permissionIndex);
  await expect(page.locator('#status')).toHaveText('Recording audio (recognizer not connected)');
  await expect.poll(() => page.evaluate(() => window.audioProbe.audioFrames)).toBeGreaterThan(0);
  await expect(screen(page)).toContainText('synthetic speech');
}

test('real worklet PCM uses dedicated ingestion and fake text preserves the draft', async ({page}, testInfo) => {
  const diagnostics = [];
  page.on('pageerror', error => diagnostics.push(error.message));
  page.on('console', message => { if (['error', 'warn'].includes(message.type())) diagnostics.push(message.text()); });
  await prepareAudioDraft(page);
  await startCapture(page, 0);
  await expect.poll(() => page.evaluate(() => window.audioProbe.audioFrames)).toBeGreaterThan(0);
  expect(await page.evaluate(() => window.audioProbe.terminalAudioFrames)).toBe(0);
  expect(await page.evaluate(() => window.audioProbe.capabilityInURL)).toBe(false);
  await expect(screen(page)).toContainText('typed draft');
  expect((await screen(page).innerText()).match(/synthetic speech/g)).toHaveLength(1);
  if (process.env.THOUGHTS_BROWSER_ARTIFACTS) await page.screenshot({path: `${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-recording.png`});
  await page.setViewportSize({width: 400, height: 850});
  await expect(screen(page)).toContainText('F8: Stop');
  await expect(screen(page)).toContainText('typed draft');
  if (process.env.THOUGHTS_BROWSER_ARTIFACTS) await page.screenshot({path: `${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-narrow-recording.png`});
  await page.keyboard.press('F8');
  await expect(screen(page)).toContainText('F8: Record');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.tracksStopped)).toBe(1);
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  await page.keyboard.type(' edited');
  await expect(screen(page)).toContainText('edited');
  if (process.env.THOUGHTS_BROWSER_ARTIFACTS) await page.screenshot({path: `${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-stopped.png`});
  expect(diagnostics).toEqual([]);
});

test('restart owns a fresh audio connection and reload releases both recording identities', async ({page}) => {
  await prepareAudioDraft(page); await startCapture(page, 0);
  await page.keyboard.press('F8'); await expect(screen(page)).toContainText('F8: Record');
  await page.keyboard.type(' between recordings');
  await startCapture(page, 1);
  expect(await page.evaluate(() => window.audioProbe.audioConnections)).toBe(2);
  await expect(screen(page)).toContainText('between recordings');
  await expect.poll(async () => ((await screen(page).innerText()).match(/synthetic speech/g) || []).length).toBe(2);
  await page.reload(); await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Events');
  await expect(screen(page)).not.toContainText('typed draft');
});

test('permission denial preserves the draft and does not open an audio socket', async ({page}) => {
  await prepareAudioDraft(page); await page.keyboard.press('F8');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.evaluate(() => window.voiceProbe.permissions[0].reject({name: 'NotAllowedError', message: 'PRIVATE-PERMISSION'}));
  await expect(screen(page)).toContainText('permission was denied');
  await expect(screen(page)).toContainText('typed draft');
  expect(await page.evaluate(() => window.audioProbe.audioConnections)).toBe(0);
});
