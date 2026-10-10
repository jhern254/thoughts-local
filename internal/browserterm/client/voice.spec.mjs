import {fakeMicrophone} from './capture-fixture.mjs';
import {test, expect} from '@playwright/test';
const screen = page => page.locator('.xterm-rows');
async function ready(page) {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('t: create thought');
  await page.keyboard.press('t');
  await expect(screen(page).locator('div').first()).toContainText('Thoughts');
  await expect(screen(page)).toContainText('Create thought');
  await expect(screen(page)).not.toContainText('Voice thought');
}
async function paste(page, content) {
  await page.evaluate(content => {
    const event = new Event('paste', {bubbles: true, cancelable: true});
    Object.defineProperty(event, 'clipboardData', {value: {getData: () => content}});
    document.getElementById('terminal').dispatchEvent(event);
  }, content);
}
test('voice capture locks editing and save; Stop and restart preserve edited draft', async ({page}) => {
  const draft = `unsaved capture ${Date.now()}`;
  await fakeMicrophone(page); await ready(page);
  await page.keyboard.type('v');
  await expect(screen(page)).toContainText('v');
  await expect(screen(page)).toContainText('F8: Record');
  await paste(page, `${draft}\n界 café 👩‍💻`);
  await expect(screen(page)).toContainText(draft);
  await page.keyboard.press('F8');
  await expect(screen(page)).toContainText('F8: Stop');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.evaluate(() => window.voiceProbe.grant(0));
  await expect(page.locator('#status')).toHaveText('Transcribing…');
  await expect(screen(page)).toContainText('F8: Stop');
  await paste(page, 'PRIVATE-MUTATION');
  await page.locator('.xterm-helper-textarea').focus();
  await page.keyboard.press('Control+s'); await page.keyboard.press('Tab');
  await expect(screen(page)).not.toContainText('PRIVATE-MUTATION');
  await expect(screen(page)).not.toContainText('Create subject…');
  await expect(screen(page)).toContainText(draft);
  await page.keyboard.press('F8');
  await expect(screen(page)).toContainText('F8: Record');
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  await paste(page, ' edited'); await expect(screen(page)).toContainText('edited');
  await page.keyboard.press('F8');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(2);
  await page.evaluate(() => window.voiceProbe.grant(1));
  await expect(page.locator('#status')).toHaveText('Transcribing…');
  await expect(screen(page)).toContainText('F8: Stop');
  await page.reload(); await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Events'); await expect(screen(page)).not.toContainText(draft);
});
test('cancel pending permission stops a late stream without reviving capture', async ({page}) => {
  await fakeMicrophone(page); await ready(page);
  await page.keyboard.press('F8');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.locator('.xterm-helper-textarea').focus(); await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('Events');
  await page.evaluate(() => window.voiceProbe.grant(0));
  await expect.poll(() => page.evaluate(() => window.voiceProbe.tracksStopped)).toBe(1);
  expect(await page.evaluate(() => window.voiceProbe.recorders.length)).toBe(0);
});
test('twenty-minute deadline stops capture without counting chunks', async ({page}) => {
  await fakeMicrophone(page); await page.clock.install(); await ready(page);
  await page.keyboard.press('F8');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.evaluate(() => window.voiceProbe.grant(0));
  await expect(page.locator('#status')).toHaveText('Transcribing…'); await expect(screen(page)).toContainText('F8: Stop');
  await page.clock.fastForward(3 * 60 * 1000);
  await expect(screen(page)).toContainText('F8: Stop');
  await page.clock.fastForward(17 * 60 * 1000);
  await expect(screen(page)).toContainText('F8: Record');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.tracksStopped)).toBe(1);
  await paste(page, 'after deadline');
  await expect(screen(page)).toContainText('after deadline');
});
test('subject picker and creation preserve Unicode draft through narrow resize', async ({page}) => {
  await ready(page); await paste(page, 'subject draft 界'); await page.keyboard.press('Tab');
  await paste(page, `Voice subject ${Date.now()}`); await page.keyboard.press('Enter');
  await expect(screen(page).locator('div').first()).toHaveText('Create subject'); await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('subject draft 界'); await page.keyboard.press('Enter');
  await expect(screen(page).locator('div').first()).toHaveText('Create subject'); await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Voice subject');
  await page.setViewportSize({width: 620, height: 700});
  await expect(screen(page)).toContainText('subject draft 界'); await expect(screen(page)).toContainText('Ctrl+S');
  await page.keyboard.press('Control+s'); await expect(screen(page)).toContainText('Thought ');
  await expect(screen(page)).toContainText('subject draft 界');
});


test('regular thought creation uses the same form and recording action', async ({page}) => {
  await fakeMicrophone(page);
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await page.keyboard.press('Tab');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Create thought…');
  await expect(screen(page)).not.toContainText('Loading…');
  await page.keyboard.press('Enter');
  await expect(screen(page).locator('div').first()).toContainText('Thoughts');
  await expect(screen(page)).toContainText('F8: Record');
  await expect(screen(page)).toContainText('Misc');
  await expect(screen(page)).not.toContainText('Voice thought');
  await expect(page.locator('#voice-controls')).toHaveCount(0);
  await page.keyboard.press('F8');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.evaluate(() => window.voiceProbe.grant(0));
  await expect(page.locator('#status')).toHaveText('Transcribing…');
  await expect(screen(page)).toContainText('F8: Stop');
  await expect(screen(page)).not.toContainText('F8: Record');
  await page.keyboard.press('F8');
  await expect(screen(page)).toContainText('F8: Record');
  await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('Create thought…');
  await expect(screen(page)).not.toContainText('F8: Record');
});
