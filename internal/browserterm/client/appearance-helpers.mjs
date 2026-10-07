import {expect} from '@playwright/test';
export const screen = page => page.locator('.xterm-rows');
export async function openAppearance(page) {
  await page.keyboard.press('Control+,');
  await expect(screen(page)).toContainText('Background darkness:');
  await expect(screen(page)).not.toContainText('Loading appearance');
}
export async function clickControl(page, label) {
  const point = await screen(page).evaluate((el, label) => {
    const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
      const node = walker.currentNode;
      if (node.textContent.trim() !== label) continue;
      const index = node.textContent.indexOf(label);
      if (index < 0) continue;
      const range = document.createRange();
      range.setStart(node, index); range.setEnd(node, index + label.length);
      const rect = range.getBoundingClientRect();
      return {x: rect.x + rect.width / 2, y: rect.y + rect.height / 2};
    }
    throw new Error('Missing TUI control');
  }, label);
  await page.mouse.click(point.x, point.y);
}
export async function setDarkness(page, value) {
  await page.keyboard.press('Tab');
  await page.keyboard.press('Home');
  for (let n = 0; n < value; n += 5) await page.keyboard.press('ArrowRight');
  await expect(screen(page)).toContainText(`Background darkness: ${value}%`);
}
export async function chooseImage(page, image) {
  const chooser = page.waitForEvent('filechooser');
  await clickControl(page, 'Choose image');
  await (await chooser).setFiles(image);
  await expect(screen(page)).toContainText('Preview only. Apply to save.');
  await expect(page.locator('#background-preview-wrap')).toBeVisible();
}
