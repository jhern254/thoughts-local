import {test, expect} from '@playwright/test';
const screen = page => page.locator('.xterm-rows');
async function fakeMicrophone(page) {
  await page.addInitScript(() => {
    window.voiceProbe = {requests: 0, tracksStopped: 0, recorders: [], permissions: []};
    Object.defineProperty(navigator, 'mediaDevices', {value: {getUserMedia: () => {
      window.voiceProbe.requests++;
      return new Promise((resolve, reject) => window.voiceProbe.permissions.push({resolve, reject}));
    }}});
    window.voiceProbe.grant = index => window.voiceProbe.permissions[index].resolve({getTracks: () => [{stop: () => { window.voiceProbe.tracksStopped++; }}]});
    window.MediaRecorder = class {
      static isTypeSupported() { return true; }
      constructor() { this.state = 'inactive'; window.voiceProbe.recorders.push(this); }
      start(interval) { this.state = 'recording'; this.interval = interval; }
      stop() { this.state = 'inactive'; queueMicrotask(() => this.onstop?.()); }
    };
  });
}
async function ready(page) {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(page.getByRole('button', {name: 'Voice input'})).toBeEnabled();
  await page.getByRole('button', {name: 'Voice input'}).click();
  await expect(screen(page)).toContainText('Voice thought');
}
async function paste(page, content) {
  await page.evaluate(content => {
    const event = new Event('paste', {bubbles: true, cancelable: true});
    Object.defineProperty(event, 'clipboardData', {value: {getData: () => content}});
    document.getElementById('terminal').dispatchEvent(event);
  }, content);
}
test('voice capture locks editing and save; Stop and restart preserve edited draft', async ({page}) => {
  await fakeMicrophone(page); await ready(page);
  await paste(page, 'voice draft\n界 café 👩‍💻');
  await expect(screen(page)).toContainText('voice draft');
  await page.getByRole('button', {name: 'Record', exact: true}).click();
  await expect(screen(page)).toContainText('Requesting microphone');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.evaluate(() => window.voiceProbe.grant(0));
  await expect(screen(page)).toContainText('Recording');
  await paste(page, 'PRIVATE-MUTATION');
  await page.locator('.xterm-helper-textarea').focus();
  await page.keyboard.press('Control+s'); await page.keyboard.press('Tab');
  await expect(screen(page)).not.toContainText('PRIVATE-MUTATION');
  await expect(screen(page)).not.toContainText('Search subjects');
  await expect(screen(page)).toContainText('voice draft');
  await page.getByRole('button', {name: 'Stop', exact: true}).click();
  await expect(screen(page)).toContainText('Stopped');
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  await paste(page, ' edited'); await expect(screen(page)).toContainText('edited');
  await page.getByRole('button', {name: 'Record', exact: true}).click();
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(2);
  await page.evaluate(() => window.voiceProbe.grant(1));
  await expect(screen(page)).toContainText('Recording');
  await page.reload(); await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Events'); await expect(screen(page)).not.toContainText('voice draft');
});
test('cancel pending permission stops a late stream without reviving capture', async ({page}) => {
  await fakeMicrophone(page); await ready(page);
  await page.getByRole('button', {name: 'Record', exact: true}).click();
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.locator('.xterm-helper-textarea').focus(); await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('Events');
  await page.evaluate(() => window.voiceProbe.grant(0));
  await expect.poll(() => page.evaluate(() => window.voiceProbe.tracksStopped)).toBe(1);
  expect(await page.evaluate(() => window.voiceProbe.recorders.length)).toBe(0);
});
test('twenty-minute deadline stops capture without counting chunks', async ({page}) => {
  await fakeMicrophone(page); await page.clock.install(); await ready(page);
  await page.getByRole('button', {name: 'Record', exact: true}).click();
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.evaluate(() => window.voiceProbe.grant(0)); await expect(screen(page)).toContainText('Recording');
  await page.clock.fastForward(3 * 60 * 1000);
  await expect(page.getByRole('button', {name: 'Stop', exact: true})).toBeEnabled();
  await expect(page.locator('#voice-time')).toContainText('03:');
  await page.clock.fastForward(17 * 60 * 1000);
  await expect(screen(page)).toContainText('Stopped');
  await expect(page.locator('#voice-time')).toHaveText('20:00 / 20:00');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.tracksStopped)).toBe(1);
  await expect(page.getByRole('button', {name: 'Record', exact: true})).toBeEnabled();
});
test('subject picker and creation preserve Unicode draft through narrow resize', async ({page}) => {
  await ready(page); await paste(page, 'subject draft 界'); await page.keyboard.press('Tab');
  await paste(page, `Voice subject ${Date.now()}`); await page.keyboard.press('ArrowDown'); await page.keyboard.press('Enter');
  await expect(screen(page).locator('div').first()).toHaveText('Create subject'); await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('subject draft 界'); await page.keyboard.press('Enter');
  await expect(screen(page).locator('div').first()).toHaveText('Create subject'); await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Subject: Voice subject');
  await page.setViewportSize({width: 620, height: 700});
  await expect(screen(page)).toContainText('subject draft 界'); await expect(screen(page)).toContainText('Ctrl+S');
  await page.keyboard.press('Control+s'); await expect(screen(page)).toContainText('Thought ');
  await expect(screen(page)).toContainText('subject draft 界');
});
