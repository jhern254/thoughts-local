import {test,expect} from '@playwright/test';
import {fileURLToPath} from 'node:url';
import {screen,clickControl,openVisual,chooseImage} from './visual-helpers.mjs';

async function ready(page) {
 await page.goto('/');
 await expect(page.locator('#status')).toHaveText('Connected');
 await expect(screen(page)).toContainText('Events');
}
async function reset(page) {
 await ready(page);
 await page.evaluate(async()=>{
  const response=await fetch('/visual/accessibility',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({thickerCardBorders:false,heavierText:false})});
  if(!response.ok) throw new Error('could not reset synthetic preferences');
 });
 await ready(page);
}
async function openAccessibility(page) {
 await page.keyboard.press('Control+,');
 await expect(screen(page)).toContainText('Accessibility');
 await clickControl(page,'Accessibility');
 await expect(screen(page)).toContainText('Thicker card borders:');
 await expect(screen(page)).not.toContainText('Loading accessibility');
}
async function weight(page,expected) {
 await expect.poll(()=>screen(page).locator('span:not(.xterm-bold)').first().evaluate(e=>getComputedStyle(e).fontWeight)).toBe(String(expected));
}

test('accessibility previews independently, cancels, saves, and restores after reload',async({page},testInfo)=>{
 await reset(page);
 await weight(page,400);
 await openAccessibility(page);
 await page.keyboard.press('ArrowRight');
 await expect(screen(page)).toContainText('Thicker card borders: On');
 await expect(screen(page)).toContainText('Heavier text: Off');
 await expect(screen(page)).toContainText('┏');
 await page.keyboard.press('ArrowDown');
 await page.keyboard.press('ArrowRight');
 await weight(page,600);
 await page.keyboard.press('Escape');
 await weight(page,400);
 await page.keyboard.press('Escape');
 await expect(screen(page)).toContainText('Events');
 await expect(screen(page)).not.toContainText('┏');
 await openAccessibility(page);
 await clickControl(page,'Heavier text: Off');
 await page.route('**/visual/accessibility',route=>route.fulfill({status:500,body:'Could not save.'}),{times:1});
 await clickControl(page,'Apply');
 await expect(screen(page)).toContainText('Could not load or save accessibility settings');
 await clickControl(page,'Back');
 await weight(page,400);
 await page.keyboard.press('Escape');
 await openAccessibility(page);
 await expect(screen(page)).toContainText('Thicker card borders: Off');
 await clickControl(page,'Thicker card borders: Off');
 await clickControl(page,'Apply');
 await expect(screen(page)).toContainText('Events');
 await expect(screen(page)).toContainText('┏');
 await weight(page,400);
 const stored=await (await page.request.get('/visual')).json();
 expect(stored.thickerCardBorders).toBe(true);
 await ready(page);
 await expect(screen(page)).toContainText('┏');
 await openAccessibility(page);
 await expect(screen(page)).toContainText('Thicker card borders: On');
 await clickControl(page,'Heavier text: Off');
 await clickControl(page,'Apply');
 await expect(screen(page)).toContainText('Events');
 await weight(page,600);
 await ready(page);
 await expect(screen(page)).toContainText('┏');
 await weight(page,600);
 await page.screenshot({path:testInfo.outputPath('saved-accessibility.png')});
 await page.setViewportSize({width:500,height:850});
 await openAccessibility(page);
 await expect(screen(page)).toContainText('Apply');
 await expect(screen(page)).toContainText('Back');
 await page.screenshot({path:testInfo.outputPath('narrow-accessibility.png')});
});

test('accessibility remains readable over video and survives background removal',async({page},testInfo)=>{
 test.setTimeout(90000);
 await reset(page);
 await openVisual(page);
 await chooseImage(page,{path:fileURLToPath(new URL('fixtures/visual-video-colors.mp4',import.meta.url)),mimeType:'video/mp4'});
 await clickControl(page,'Apply');
 await expect(screen(page)).toContainText('Events');
 await expect(page.locator('#background-video')).toBeVisible();
 await page.screenshot({path:testInfo.outputPath('video-normal.png')});
 await openAccessibility(page);
 await clickControl(page,'Thicker card borders: Off');
 await clickControl(page,'Heavier text: Off');
 await clickControl(page,'Apply');
 await expect(screen(page)).toContainText('Events');
 await expect(screen(page)).toContainText('┏');
 await weight(page,600);
 for(const darkness of [20,80]) {
  await openVisual(page);
  const settings=await (await page.request.get('/visual')).json();
  await clickControl(page,`Background darkness: ${settings.darkness}%`);
  await page.keyboard.press('Home');
  for(let i=0;i<darkness;i++) await page.keyboard.press('ArrowRight');
  await clickControl(page,'Apply');
  await expect(screen(page)).toContainText('Events');
  await expect(screen(page)).toContainText('┏');
  await weight(page,600);
  await page.screenshot({path:testInfo.outputPath(`video-accessibility-${darkness}.png`)});
 }
 await openVisual(page);
 await clickControl(page,'Remove background');
 await expect(page.locator('#background-video')).toHaveCount(0);
 await expect(screen(page)).toContainText('┏');
 await weight(page,600);
});
