import {test, expect} from '@playwright/test';

async function imageFixture(page) {
  const encoded = await page.evaluate(() => {
    const canvas = document.createElement('canvas');
    canvas.width = 1200; canvas.height = 850;
    const ctx = canvas.getContext('2d');
    const gradient = ctx.createLinearGradient(0, 0, 1200, 850);
    gradient.addColorStop(0, '#164f93'); gradient.addColorStop(0.5, '#ce66a1'); gradient.addColorStop(1, '#ce9b34');
    ctx.fillStyle = gradient; ctx.fillRect(0, 0, 1200, 850);
    ctx.fillStyle = '#68c8b9'; ctx.beginPath(); ctx.arc(950, 260, 210, 0, 2 * Math.PI); ctx.fill();
    return canvas.toDataURL('image/png').split(',')[1];
  });
  return {name: 'synthetic.png', mimeType: 'image/png', buffer: Buffer.from(encoded, 'base64')};
}
async function open(page) {
  await page.keyboard.press('Control+,');
  await expect(page.locator('#appearance-dialog')).toBeVisible();
  await expect(page.locator('#appearance-apply')).toBeEnabled();
}
async function darkness(page, value) {
  await page.locator('#background-darkness').fill(String(value));
  await page.locator('#background-darkness').dispatchEvent('input');
}
test('persists one image without resetting the terminal or its draft', async ({page}, testInfo) => {
  let sockets = 0;
  page.on('websocket', () => sockets++);
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(page.locator('.xterm-rows')).toContainText('Options');
  await page.keyboard.press('Tab');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('Enter');
  await expect(page.locator('#appearance-dialog')).toBeVisible();
  await expect(page.locator('#appearance-apply')).toBeEnabled();
  await page.getByRole('button', {name: 'Remove background'}).click();
  await expect(page.locator('#appearance-dialog')).not.toBeVisible();
  await expect(page.locator('#background-image')).toBeHidden();
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-default.png`});
  await page.keyboard.press('t');
  await expect(page.locator('.xterm-rows')).toContainText('Create thought');
  await page.keyboard.insertText('Appearance keeps this unsaved draft 界');
  await open(page);
  await page.locator('#background-file').setInputFiles(await imageFixture(page));
  await expect(page.locator('#background-preview')).toBeVisible();
  await darkness(page, 70);
  await page.getByRole('button', {name: 'Apply', exact: true}).click();
  await expect(page.locator('#appearance-dialog')).not.toBeVisible();
  await expect(page.locator('#background-image')).toBeVisible();
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  await expect(page.locator('.xterm-rows')).toContainText('Appearance keeps this unsaved draft 界');
  expect(sockets).toBe(1);
  expect(await page.locator('.xterm-viewport').evaluate(el => getComputedStyle(el).backgroundColor)).toBe('rgba(0, 0, 0, 0)');
  expect(await page.locator('.xterm-scrollable-element').evaluate(el => getComputedStyle(el).backgroundColor)).toBe('rgba(0, 0, 0, 0)');
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-image.png`});
  await open(page);
  await darkness(page, 30);
  await page.getByRole('button', {name: 'Apply', exact: true}).click();
  await expect(page.locator('#appearance-dialog')).not.toBeVisible();
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-low.png`});
  await open(page);
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-options.png`});
  await darkness(page, 90);
  await page.getByRole('button', {name: 'Apply', exact: true}).click();
  await expect(page.locator('#appearance-dialog')).not.toBeVisible();
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-high.png`});
  await page.setViewportSize({width: 620, height: 700});
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-narrow.png`});
  await expect(page.locator('.xterm-rows')).toContainText('unsaved draft');
  expect(sockets).toBe(1);
  await page.reload();
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(page.locator('#background-image')).toBeVisible();
  await open(page);
  await expect(page.locator('#background-darkness')).toHaveValue('90');
  await darkness(page, 10);
  await page.keyboard.press('Escape');
  await expect(page.locator('#appearance-dialog')).not.toBeVisible();
  await expect(page.locator('#background-darkness')).toHaveValue('90');
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  await open(page);
  await page.getByRole('button', {name: 'Remove background'}).click();
  await expect(page.locator('#background-image')).toBeHidden();
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
});
test('failed replacement retains the active image', async ({page}) => {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await open(page);
  await page.locator('#background-file').setInputFiles(await imageFixture(page));
  await page.getByRole('button', {name: 'Apply', exact: true}).click();
  await expect(page.locator('#background-image')).toBeVisible();
  const before = await page.request.get('/appearance/background');
  await open(page);
  await page.locator('#background-file').setInputFiles(await imageFixture(page));
  await page.route('**/appearance/background', async route => {
    if (route.request().method() === 'POST') await route.fulfill({status: 500, body: 'Could not update or load the background.'});
    else await route.continue();
  });
  await page.getByRole('button', {name: 'Apply', exact: true}).click();
  await expect(page.locator('#appearance-status')).toContainText('Could not apply');
  const after = await page.request.get('/appearance/background');
  expect(await after.body()).toEqual(await before.body());
  await expect(page.locator('#background-image')).toBeVisible();
  await page.getByRole('button', {name: 'Remove background'}).click();
  await expect(page.locator('#background-image')).toBeHidden();
});
