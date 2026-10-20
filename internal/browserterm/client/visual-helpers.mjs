import {expect} from '@playwright/test';
export const screen = page => page.locator('.xterm-rows');
export async function openVisual(page) {
  await page.keyboard.press('Control+,');
  await expect(screen(page)).toContainText(/Background darkness:|Models/);
  if (!(await screen(page).textContent()).includes('Background darkness:')) await clickControl(page,'Visual');
  await expect(screen(page)).toContainText('Background darkness:');
  await expect(screen(page)).not.toContainText('Loading visual');
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
  await page.keyboard.press('Tab');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Home');
  for (let n = 0; n < value; n++) await page.keyboard.press('ArrowRight');
  await expect(screen(page)).toContainText(`Background darkness: ${value}%`);
}
export async function chooseImage(page, image) {
  const chooser = page.waitForEvent('filechooser');
  await clickControl(page, 'Choose image or video');
  await (await chooser).setFiles(image);
  if (image.mimeType?.startsWith('video/')) {
    await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
    await clickControl(page,'Preview loop');
    await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
    await clickControl(page,'Done — use previewed loop');
    await expect(screen(page)).toContainText('Background darkness:');
  } else {
    await expect(screen(page)).toContainText('Preview only. Apply to save.');
  }
  await expect(page.locator('#background-preview-wrap')).toBeVisible();
}

export async function setVideoQuality(page,tier) {
 const labels=['Efficient (720p30)','High quality (1080p30)','Maximum (1080p60)'];
 const index=['efficient','high','maximum'].indexOf(tier);
 if(index<0)throw new Error('Unknown test quality');
 await page.locator('.xterm-helper-textarea').focus();
 for(let step=0;step<16;step++) {
  const selected=await page.locator('.xterm-bg-62').allTextContents();
  if(selected.some(text=>text.trim().startsWith('Video quality: ')))break;
  if(step===15)throw new Error('Video quality control did not receive focus');
  await page.keyboard.press('Tab');
  await expect.poll(()=>page.locator('.xterm-bg-62').allTextContents()).not.toEqual(selected);
 }
 await page.keyboard.press('Home');
 for(let step=0;step<index;step++)await page.keyboard.press('ArrowRight');
 await expect(screen(page)).toContainText(`Video quality: ${labels[index]}`);
}
