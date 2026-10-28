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
  const response=await fetch('/visual/accessibility',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({borderWeight:'normal',heavierText:false,textSize:'standard',highContrast:false})});
  if(!response.ok) throw new Error('could not reset synthetic preferences');
 });
 await ready(page);
}
async function openAccessibility(page) {
 await page.keyboard.press('Control+,');
 await expect(screen(page)).toContainText('Accessibility');
 await clickControl(page,'Accessibility');
 await expect(screen(page)).toContainText('Border weight:');
 await expect(screen(page)).not.toContainText('Loading accessibility');
}
async function weight(page,expected) {
 await expect.poll(()=>screen(page).locator('span:not(.xterm-bold)').first().evaluate(e=>getComputedStyle(e).fontWeight)).toBe(String(expected));
}

test.afterEach(async({page})=>{
 await page.evaluate(async()=>{
  const response=await fetch('/visual/accessibility',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({borderWeight:'normal',heavierText:false,textSize:'standard',highContrast:false})});
  if(!response.ok) throw new Error('could not reset synthetic readability preferences');
 });
});

test('accessibility previews independently, cancels, saves, and restores after reload',async({page},testInfo)=>{
 await reset(page);
 await weight(page,400);
 await openAccessibility(page);
 await page.keyboard.press('ArrowRight');
 await expect(screen(page)).toContainText('Border weight: Heavy');
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
 await expect(screen(page)).toContainText('Border weight: Normal');
 await clickControl(page,'Border weight: Normal');
 await clickControl(page,'Apply');
 await expect(screen(page)).toContainText('Events');
 await expect(screen(page)).toContainText('┏');
 await weight(page,400);
 const stored=await (await page.request.get('/visual')).json();
 expect(stored.borderWeight).toBe('heavy');
 await ready(page);
 await expect(screen(page)).toContainText('┏');
 await openAccessibility(page);
 await expect(screen(page)).toContainText('Border weight: Heavy');
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
 await clickControl(page,'Border weight: Normal');
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

test('readability sizes contrast and structural dividers preview save and restore',async({page},testInfo)=>{
 await reset(page);
 const originalDarkness=(await (await page.request.get('/visual')).json()).darkness;
 await openAccessibility(page);
 await page.keyboard.press('ArrowLeft');
 await expect(screen(page)).toContainText('Border weight: Heavy');
 await expect(screen(page)).toContainText('┏');
 await page.keyboard.press('ArrowDown');
 await page.keyboard.press('ArrowDown');
 await page.keyboard.press('ArrowRight');
 await expect(screen(page)).toContainText('Text size: Larger');
 await expect.poll(()=>screen(page).locator('span').first().evaluate(e=>getComputedStyle(e).fontSize)).toBe('16px');
 await page.keyboard.press('ArrowDown');
 await page.keyboard.press('ArrowRight');
 await expect(screen(page)).toContainText('High contrast: On');
 await expect(page.locator('body')).toHaveClass(/high-contrast/);
 await expect.poll(()=>page.locator('#background-overlay').evaluate(e=>e.style.backgroundColor)).toBe('rgba(0, 0, 0, 0.85)');
 await page.keyboard.press('Escape');
 await expect.poll(()=>screen(page).locator('span').first().evaluate(e=>getComputedStyle(e).fontSize)).toBe('14px');
 await expect(page.locator('body')).not.toHaveClass(/high-contrast/);
 await page.keyboard.press('Escape');
 await expect(screen(page)).toContainText('Events');
 await openAccessibility(page);
 await clickControl(page,'Border weight: Normal');
 await clickControl(page,'Text size: Standard');
 await clickControl(page,'High contrast: Off');
 await clickControl(page,'Apply');
 await expect(screen(page)).toContainText('Events');
 // Section dividers span the terminal; cards alone must not satisfy this.
 await expect(screen(page)).toContainText(/━{20}/);
 const stored=await (await page.request.get('/visual')).json();
 expect(stored.borderWeight).toBe('heavy');
 expect(stored.textSize).toBe('larger');
 expect(stored.highContrast).toBe(true);
 expect(stored.darkness).toBe(originalDarkness);
 await ready(page);
 await expect(screen(page)).toContainText(/━{20}/);
 await expect.poll(()=>screen(page).locator('span').first().evaluate(e=>getComputedStyle(e).fontSize)).toBe('16px');
 await page.screenshot({path:testInfo.outputPath('larger-heavy.png')});
 // A short viewport forces the controls to scroll without losing Back/Apply.
 await page.setViewportSize({width:420,height:240});
 await openAccessibility(page);
 await page.keyboard.press('ArrowUp');
 await expect(screen(page)).toContainText('Back');
 await page.screenshot({path:testInfo.outputPath('short-accessibility.png')});
 await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('Options');
 await page.setViewportSize({width:1200,height:850});
 await page.keyboard.press('Escape');
 await openVisual(page);
 await expect(page.locator('#background-preview-wrap')).toBeHidden();
 // Apply/Back and source geometry still work at the saved Larger size.
 await expect(screen(page)).toContainText('Choose image or video');
 const chooser=page.waitForEvent('filechooser');
 await clickControl(page,'Choose image or video');
 await(await chooser).setFiles(fileURLToPath(new URL('fixtures/visual-video-colors.mp4',import.meta.url)));
 await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
 await expect(page.locator('#background-preview-wrap')).toBeVisible();
 await page.screenshot({path:testInfo.outputPath('heavy-source-frame.png')});
 // The trimmer scrolls focused controls into view at the larger font size.
 for(let i=0;i<4;i++) await page.keyboard.press('Tab');
 await clickControl(page,'Preview loop');
 await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await page.keyboard.press('Tab');
 await clickControl(page,'Done — use previewed loop');
 await expect(page.locator('#background-preview-wrap')).toBeVisible();
 await page.screenshot({path:testInfo.outputPath('heavy-media-frame.png')});
 await page.keyboard.press('Escape');
});


test('quarter-window Larger text keeps Events Thoughts and Options usable without resizing',async({page},testInfo)=>{
 let sockets=0;
 page.on('websocket',()=>sockets++);
 await page.setViewportSize({width:960,height:450});
 await reset(page);
 await openAccessibility(page);
 await clickControl(page,'Text size: Standard');
 await expect(screen(page)).toContainText('Text size: Larger');
 await expect(screen(page)).not.toContainText('Largest');
 await clickControl(page,'Apply');
 await expect(screen(page)).toContainText('Now ·');
 await expect(screen(page)).not.toContainText('Resize terminal');
 await expect.poll(()=>screen(page).locator('span').first().evaluate(e=>getComputedStyle(e).fontSize)).toBe('16px');
 await page.screenshot({path:testInfo.outputPath('quarter-events-larger.png')});
 const activeSockets=sockets;
 await page.keyboard.press('ArrowUp');
 await expect(screen(page)).not.toContainText('Resize terminal');
 await page.keyboard.press('End');
 await page.keyboard.press('t');
 await expect(screen(page)).toContainText('Create thought');
 await page.keyboard.type('Quarter-window thought');
 await page.keyboard.press('Enter');
 await page.keyboard.type('Second line stays visible');
 await expect(screen(page)).toContainText('Quarter-window thought');
 await expect(screen(page)).toContainText('Second line stays visible');
 await expect(screen(page)).toContainText('Ctrl+S');
 await page.screenshot({path:testInfo.outputPath('quarter-thought-larger.png')});
 await page.keyboard.press('Control+q');
 await expect(screen(page)).toContainText('Discard draft?');
 await page.keyboard.press('Tab');await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('Events');
 await openAccessibility(page);
 await expect(screen(page)).toContainText('Apply');
 await expect(screen(page)).toContainText('Back');
 const screenBottom=await page.locator('.xterm-screen').evaluate(e=>e.getBoundingClientRect().bottom);
 for(const label of ['Apply','Back']){
  const bounds=await screen(page).getByText(label,{exact:true}).boundingBox();
  expect(bounds.y+bounds.height).toBeLessThanOrEqual(screenBottom+1);
 }
 await page.screenshot({path:testInfo.outputPath('quarter-options-larger.png')});
 await clickControl(page,'Back');
 await clickControl(page,'Visual');
 await expect(screen(page)).toContainText('Choose image or video');
 await expect(screen(page)).toContainText('Apply');
 await expect(screen(page)).toContainText('Back');
 await page.screenshot({path:testInfo.outputPath('quarter-visual-larger.png')});
 const chooser=page.waitForEvent('filechooser');
 await clickControl(page,'Choose image or video');
 await(await chooser).setFiles(fileURLToPath(new URL('fixtures/visual-video-colors.mp4',import.meta.url)));
 await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
 await expect(screen(page)).not.toContainText('Resize window');
 const preview=page.locator('#background-preview-wrap');
 await expect(preview).toBeVisible();
 const quarterPreview=await preview.boundingBox();
 await page.screenshot({path:testInfo.outputPath('quarter-trimmer-larger.png')});
 for(let i=0;i<4;i++) await page.keyboard.press('Tab');
 await clickControl(page,'Preview loop');
 await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await expect(screen(page)).toContainText('Range:');
 await expect.poll(async()=>(await preview.boundingBox())?.height).toBe(quarterPreview.height);
 await page.setViewportSize({width:1200,height:850});
 await expect.poll(async()=>(await preview.boundingBox())?.height).toBe(quarterPreview.height);
 await page.setViewportSize({width:960,height:450});
 await expect(preview).toBeVisible();
 await expect(screen(page)).toContainText('Done — use previewed loop');
 await page.screenshot({path:testInfo.outputPath('quarter-trimmer-ready.png')});
 await clickControl(page,'Done — use previewed loop');
 await expect(screen(page)).toContainText('Background darkness:');
 expect(sockets).toBe(activeSockets);
});
