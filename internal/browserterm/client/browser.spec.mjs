import {test, expect} from '@playwright/test';

const screen = page => page.locator('.xterm-rows');
async function ready(page) {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Events');
}

test('renders newlines at the same column as native terminal output', async ({page}) => {
  await ready(page);
  // A pipe does not apply the PTY's ONLCR output translation. Without the
  // corresponding xterm option, each new line incorrectly moves to the right.
  await expect.poll(() => screen(page).locator('div').nth(2).textContent()).toMatch(/^ Events/);
});

test('second tab is busy; quit and reload start independent sessions', async ({page, context}) => {
  await ready(page);
  const second = await context.newPage();
  await second.goto('/');
  await expect(second.locator('#status')).toContainText('Another tab is active');
  await page.keyboard.press('Control+c');
  await expect(page.locator('#status')).toContainText('Session ended');
  await second.getByRole('button', {name: 'Reconnect'}).click();
  await expect(second.locator('#status')).toHaveText('Connected');
  await expect(screen(second)).toContainText('Events');
  await second.reload();
  await expect(second.locator('#status')).toHaveText('Connected');
  await expect(screen(second)).toContainText('Events');
});

test('Ctrl+C copies a terminal selection and quits only when there is no selection', async ({page}) => {
  await ready(page);
  const box = await screen(page).locator('div').first().boundingBox();
  await page.mouse.move(box.x + 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + 100, box.y + box.height / 2, {steps: 10});
  await page.mouse.up();
  await expect(page.locator('.xterm-selection div').first()).toBeVisible();
  await page.keyboard.press('Control+c');
  await expect(page.locator('#status')).toHaveText('Connected');
  await page.mouse.click(box.x + 3, box.y + box.height / 2);
  await page.keyboard.press('Control+c');
  await expect(page.locator('#status')).toContainText('Session ended');
});

test('browser paste preserves Unicode and rejects unsupported content without deleting selection', async ({page}) => {
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('console', message => { if (['error','warn'].includes(message.type())) errors.push(message.text()); });
  await ready(page);
  await page.keyboard.press('Tab');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Misc thoughts');
  await expect(screen(page)).not.toContainText('Loading…');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Create thought');
  await expect(screen(page)).not.toContainText('Loading…');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Ctrl+S');
  await page.keyboard.insertText('λ');
  await expect(screen(page)).toContainText('λ');
  await page.keyboard.press('Control+g');
  const paste = async text => {
    await page.evaluate(text => navigator.clipboard.writeText(text), text);
    await page.keyboard.press('Control+v');
  };
  await paste('browser draft\n界 café 👩‍💻');
  await expect(screen(page)).toContainText('browser draft');
  await expect(screen(page)).toContainText('界 café 👩‍💻');
  expect((await screen(page).innerText()).match(/browser draft/g)).toHaveLength(1);
  await page.keyboard.press('Control+g');
  await paste('bad\r\nPRIVATE-PASTE-MARKER\t\ufffd');
  await expect(screen(page)).toContainText('Input rejected');
  await expect(screen(page)).toContainText('browser draft');
  await expect(screen(page)).not.toContainText('PRIVATE-PASTE-MARKER');
  await page.setViewportSize({width: 620, height: 700});
  await expect(screen(page)).toContainText('browser draft');
  await expect(screen(page)).toContainText('Ctrl+S');
  await page.keyboard.press('Escape');
  await expect(screen(page)).not.toContainText('Ctrl+S');
  expect(errors).toEqual([]);
});
