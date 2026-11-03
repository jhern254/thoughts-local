import {test, expect} from '@playwright/test';

const screen = page => page.locator('.xterm-rows');
async function startPicker(page, activity) {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Events');
  if ((await screen(page).innerText()).includes('Enable local speech-to-text')) {
    await page.keyboard.press('Escape');
    await expect(screen(page)).not.toContainText('Enable local speech-to-text');
  }
  await page.locator('.xterm-helper-textarea').focus();
  await page.keyboard.press('n');
  await expect(screen(page)).toContainText('Start event');
  await page.keyboard.type(activity);
  await page.keyboard.press('Tab');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Select Subjects');
  await expect(screen(page)).not.toContainText('Loading subjects');
}

test('Event multiselect retains filtered selections and cancels picker edits without reconnecting', async ({page}) => {
  let connections = 0;
  page.on('websocket', () => connections++);
  const activity = `Multiple Subjects ${Date.now()}`;
  await startPicker(page, activity);
  const connected = connections;
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  await expect(screen(page)).toContainText('2 selected');
  const names = (await screen(page).innerText()).split('\n').filter(line => line.includes('[x] ')).map(line => line.split('[x] ')[1].trim());
  expect(names).toHaveLength(2);
  await page.keyboard.press('/');
  await page.keyboard.type('no-matching-subject-for-this-query');
  await expect(screen(page)).toContainText('No matching subjects');
  await expect(screen(page)).toContainText('2 selected');
  await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('Start event');
  await expect(screen(page)).toContainText(activity);
  await expect(screen(page)).toContainText('0 selected');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Select Subjects');
  await page.keyboard.press('Control+a');
  await page.keyboard.press('Control+k');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Start event');
  await expect(screen(page)).toContainText('2 selected');
  await page.setViewportSize({width: 700, height: 450});
  await expect(screen(page)).toContainText('2 selected');
  await page.keyboard.press('Control+s');
  await expect(screen(page)).not.toContainText('Start event');
  await expect(screen(page)).toContainText(activity);
  await page.keyboard.press('Enter');
  for (const name of names) await expect(screen(page)).toContainText(name);
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('←/→: Subjects');
  await page.keyboard.press('ArrowRight');
  await expect(screen(page)).toContainText('←/→: Subjects');
  await expect(screen(page)).toContainText(activity);
  await page.keyboard.press('ArrowDown');
  await expect(screen(page)).not.toContainText('←/→: Subjects');
  expect(connections).toBe(connected);
  await page.screenshot({path: `${process.env.THOUGHTS_BROWSER_ARTIFACTS}/event-subjects-${test.info().project.name}.png`});
});

test('creating a Subject preserves existing Event selections and draft', async ({page}) => {
  const activity = `Subject creation Event ${Date.now()}`;
  const subject = `Event Subject ${Date.now()}`;
  await startPicker(page, activity);
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  await page.keyboard.press('/');
  await page.keyboard.type(subject);
  await page.keyboard.press('Enter');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Subject name:');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Select Subjects');
  await expect(screen(page)).toContainText('2 selected');
  await expect(screen(page)).toContainText(subject);
  await page.keyboard.press('Control+s');
  await expect(screen(page)).toContainText(activity);
  await expect(screen(page)).not.toContainText('Start event');
});
