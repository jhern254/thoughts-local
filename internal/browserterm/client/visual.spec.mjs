import {test, expect} from '@playwright/test';
import {openVisual as open, clickControl, setDarkness as darkness, chooseImage, screen} from './visual-helpers.mjs';

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
test('persists one image without resetting the terminal or its draft', async ({page}, testInfo) => {
  let sockets = 0;
  page.on('websocket', () => sockets++);
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(page.locator('.xterm-rows')).toContainText('Options');
  await page.keyboard.press('Tab');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Background darkness:');
  await expect(screen(page)).not.toContainText('Loading visual');
  await clickControl(page, 'Remove background');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await expect(page.locator('#background-image')).toBeHidden();
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-default.png`});
  await page.keyboard.press('t');
  await expect(page.locator('.xterm-rows')).toContainText('Create thought');
  await page.keyboard.insertText('Visual keeps this unsaved draft 界');
  await open(page);
  await chooseImage(page, await imageFixture(page));
  await expect(page.locator('#background-preview')).toBeVisible();
  await darkness(page, 70);
  await clickControl(page, 'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await expect(page.locator('#background-image')).toBeVisible();
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  await expect(page.locator('.xterm-rows')).toContainText('Visual keeps this unsaved draft 界');
  expect(sockets).toBe(1);
  expect(await page.locator('.xterm-viewport').evaluate(el => getComputedStyle(el).backgroundColor)).toBe('rgba(0, 0, 0, 0)');
  expect(await page.locator('.xterm-scrollable-element').evaluate(el => getComputedStyle(el).backgroundColor)).toBe('rgba(0, 0, 0, 0)');
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-image.png`});
  await open(page);
  await darkness(page, 30);
  await clickControl(page, 'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-low.png`});
  await open(page);
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-options.png`});
  await darkness(page, 90);
  await clickControl(page, 'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-high.png`});
  await page.setViewportSize({width: 620, height: 700});
  await page.screenshot({path: `/tmp/background-${testInfo.project.name}-narrow.png`});
  await expect(page.locator('.xterm-rows')).toContainText('unsaved draft');
  expect(sockets).toBe(1);
  await page.reload();
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(page.locator('#background-image')).toBeVisible();
  await open(page);
  await expect(screen(page)).toContainText('Background darkness: 90%');
  await darkness(page, 10);
  await page.keyboard.press('Escape');
  await expect(screen(page)).not.toContainText('Background darkness:');
  expect(await page.locator('#background-overlay').evaluate(el => el.style.backgroundColor)).toBe('rgba(0, 0, 0, 0.9)');
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  await open(page);
  await clickControl(page, 'Remove background');
  await expect(page.locator('#background-image')).toBeHidden();
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
});
test('failed replacement retains the active image', async ({page}) => {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await open(page);
  await chooseImage(page, await imageFixture(page));
  await clickControl(page, 'Apply');
  await expect(page.locator('#background-image')).toBeVisible();
  const before = await page.request.get('/visual/background');
  await open(page);
  await chooseImage(page, await imageFixture(page));
  await page.route('**/visual/background', async route => {
    if (route.request().method() === 'POST') await route.fulfill({status: 500, body: 'Could not update or load the background.'});
    else await route.continue();
  });
  await clickControl(page, 'Apply');
  await expect(screen(page)).toContainText('Could not load or apply');
  const after = await page.request.get('/visual/background');
  expect(await after.body()).toEqual(await before.body());
  await expect(page.locator('#background-image')).toBeVisible();
  await clickControl(page, 'Remove background');
  await expect(page.locator('#background-image')).toBeHidden();
});

test('keyboard picker, narrow layout, dragging and resize keep preview inside its TUI border', async ({page}, testInfo) => {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await open(page);
  const chooser = page.waitForEvent('filechooser');
  await page.keyboard.press('Enter');
  await (await chooser).setFiles(await imageFixture(page));
  await expect(screen(page)).toContainText('Preview only. Apply to save.');
  for (const size of [{width:1200,height:850}, {width:620,height:700}, {width:375,height:600}, {width:620,height:400}]) {
    await page.setViewportSize(size);
    if (size.height === 400) {
      await expect(page.locator('#background-preview-wrap')).toBeHidden();
      await expect(screen(page)).toContainText('Back');
      await page.screenshot({path:`/tmp/options-${testInfo.project.name}-${size.width}x${size.height}.png`});
      continue;
    }
    await expect(page.locator('#background-preview-wrap')).toBeVisible();
    await expect(screen(page)).toContainText('Back');
    await expect.poll(async () => page.locator('#background-preview-wrap').evaluate(el => {
      const rect = el.getBoundingClientRect();
      const rows = [...document.querySelectorAll('.xterm-rows > div')];
      const top = rows.find(row => row.textContent.includes('╭'))?.getBoundingClientRect();
      const bottom = rows.find(row => row.textContent.includes('╰'))?.getBoundingClientRect();
      return Math.max(Math.abs(rect.top - top?.bottom), Math.abs(rect.bottom - bottom?.top));
    })).toBeLessThan(1);
    await page.screenshot({path:`/tmp/options-${testInfo.project.name}-${size.width}x${size.height}.png`});
  }
  // Click and drag the rendered rail itself; no HTML range input remains.
  const rail = screen(page).locator('div').filter({hasText:'●'}).first();
  const box = await rail.boundingBox();
  await page.mouse.move(box.x+box.width/2,box.y+box.height/2);
  await page.mouse.down();
  await page.mouse.move(box.x+box.width/2+30,box.y+box.height/2,{steps:4});
  await page.mouse.up();
  await expect(screen(page)).not.toContainText('Background darkness: 70%');
  await page.keyboard.press('Escape');
  await expect(page.locator('#background-preview-wrap')).toBeHidden();
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
});

test('invalid selection clears an unsaved preview without implying it was applied', async ({page}) => {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await open(page);
  await clickControl(page, 'Remove background');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await open(page);
  await chooseImage(page, await imageFixture(page));
  const chooser = page.waitForEvent('filechooser');
  await clickControl(page, 'Choose image');
  await (await chooser).setFiles({name:'invalid.png',mimeType:'image/png',buffer:Buffer.from('not an image')});
  await expect(screen(page)).toContainText('Could not load or apply');
  await expect(page.locator('#background-preview')).not.toHaveAttribute('src');
  await expect(page.locator('#background-preview-wrap')).toBeHidden();
  await clickControl(page, 'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await expect(page.locator('#background-image')).toBeHidden();
});

test('returning to the entity picker repaints behind the closed preview', async ({page}, testInfo) => {
  await page.route('**/static/client.js', async route => {
    const response = await route.fetch();
    await route.fulfill({response, body: `
      window.refreshes = [];
      window.Terminal = class extends window.Terminal {
        refresh(start, end) {
          window.refreshes.push({start, end, rows: this.rows});
          return super.refresh(start, end);
        }
      };
    ` + await response.text()});
  });
  let sockets = 0;
  page.on('websocket', () => sockets++);
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await open(page);
  await chooseImage(page, await imageFixture(page));
  await clickControl(page, 'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await page.keyboard.press('Tab');
  await page.keyboard.press('ArrowRight');
  for (const exit of ['Escape', 'Back']) {
    await page.keyboard.press('Enter');
    await expect(page.locator('#background-preview-wrap')).toBeVisible();
    await page.evaluate(() => { window.refreshes = []; });
    if (exit === 'Escape') await page.keyboard.press('Escape');
    else await clickControl(page, 'Back');
    await expect(screen(page)).not.toContainText('Background darkness:');
    await expect(page.locator('#background-preview-wrap')).toBeHidden();
    await expect.poll(() => page.evaluate(() => window.refreshes.some(r => r.start === 0 && r.end === r.rows - 1))).toBe(true);
    await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
    await expect(screen(page)).toContainText('Options');
    await page.screenshot({path: `/tmp/options-return-${testInfo.project.name}-${exit}.png`});
    expect(await page.evaluate(() => window.refreshes.length)).toBe(1);
  }
  expect(sockets).toBe(1);
});

test('frames a portrait, matches the viewport crop, and restores framing after reload', async ({page}, testInfo) => {
  let sockets = 0;
  page.on('websocket', () => sockets++);
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await open(page);
  const portrait = await page.evaluate(() => {
    const canvas = document.createElement('canvas'); canvas.width=600; canvas.height=1200;
    const context=canvas.getContext('2d');
    for (let row=0;row<6;row++) {
      context.fillStyle=['#ff4444','#ee9900','#ffff66','#33bb88','#5599ff','#9944dd'][row];
      context.fillRect(0,row*200,600,200);
      context.fillStyle='#111';context.font='60px monospace';context.fillText(`Band ${row+1}`,160,row*200+110);
    }
    return canvas.toDataURL('image/png').split(',')[1];
  });
  await chooseImage(page,{name:'portrait.png',mimeType:'image/png',buffer:Buffer.from(portrait,'base64')});
  await darkness(page,30);
  if ((await screen(page).textContent()).includes('Image fit: Fit')) await clickControl(page,'Image fit: Fit');
  await clickControl(page,'Adjust framing');
  await expect(screen(page)).toContainText('Preview · Zoom: 100%');
  const frame=page.locator('#background-preview-frame');
  await expect(frame).toHaveAttribute('data-editing','true');
  const bounds=await frame.boundingBox();
  expect(Math.abs(bounds.width/bounds.height - 1200/850)).toBeLessThan(0.01);
  await page.mouse.move(bounds.x+bounds.width/2,bounds.y+bounds.height/2);
  await page.mouse.down();
  await page.mouse.move(bounds.x+bounds.width/2,bounds.y+bounds.height/2+70,{steps:5});
  await page.mouse.up();
  for(let i=0;i<10;i++) await page.keyboard.press('+');
  await expect(screen(page)).toContainText('Zoom: 150%');
  await page.screenshot({path:`/tmp/framing-${testInfo.project.name}-portrait.png`});
  await clickControl(page,'Done');
  await expect(screen(page)).toContainText('Background darkness:');
  await expect(frame).toHaveAttribute('data-editing','false');
  const crop=await frame.evaluate(el=>{
    const image=el.querySelector('img');
    return {left:parseFloat(image.style.left)/el.clientWidth,top:parseFloat(image.style.top)/el.clientHeight,
      width:parseFloat(image.style.width)/el.clientWidth,height:parseFloat(image.style.height)/el.clientHeight};
  });
  await clickControl(page,'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  const saved=await (await page.request.get('/visual')).json();
  expect(saved.framing.zoom).toBe(150);
  expect(saved.framing.positionY).toBeLessThan(5000);
  const actual=await page.locator('#background-image').evaluate(image=>({
    left:parseFloat(image.style.left)/innerWidth,top:parseFloat(image.style.top)/innerHeight,
    width:parseFloat(image.style.width)/innerWidth,height:parseFloat(image.style.height)/innerHeight}));
  for(const key of Object.keys(crop)) expect(Math.abs(crop[key]-actual[key])).toBeLessThan(0.02);
  expect(sockets).toBe(1);
  await page.reload();
  await expect(page.locator('#status')).toHaveText('Connected');
  await open(page);await clickControl(page,'Adjust framing');
  await expect(screen(page)).toContainText('Zoom: 150%');
  await page.keyboard.press('+');
  await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('Background darkness:');
  await clickControl(page,'Adjust framing');
  await expect(screen(page)).toContainText('Zoom: 150%');
  await clickControl(page,'Reset framing');
  await expect(screen(page)).toContainText('Zoom: 100%');
  await clickControl(page,'Done');
  await expect(screen(page)).toContainText('Background darkness:');
  await clickControl(page,'Image fit: Fill');
  await expect(screen(page)).toContainText('Image fit: Fit');
  await page.screenshot({path:`/tmp/framing-${testInfo.project.name}-fit.png`});
  await clickControl(page,'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  for (const size of [{width:1200,height:850},{width:375,height:600}]) {
    await page.setViewportSize(size);
    await expect.poll(()=>page.locator('#background-image').evaluate(image=>{
      const box=image.getBoundingClientRect();
      return box.left>=-0.01 && box.top>=-0.01 && box.right<=innerWidth+0.01 && box.bottom<=innerHeight+0.01;
    })).toBe(true);
    await page.screenshot({path:`/tmp/framing-${testInfo.project.name}-fit-${size.width}.png`});
  }
});

test('Fill preserves the viewport composition for square, panoramic and thin source images', async ({page},testInfo) => {
  await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
  for (const [width,height] of [[500,500],[4096,256],[8,4096]]) {
    await open(page);
    const encoded=await page.evaluate(([width,height])=>{
      const canvas=document.createElement('canvas');canvas.width=width;canvas.height=height;
      const context=canvas.getContext('2d');
      context.fillStyle='#3388aa';context.fillRect(0,0,width,height);
      context.fillStyle='#ddbb44';context.fillRect(0,0,width/2,height/2);
      return canvas.toDataURL('image/png').split(',')[1];
    },[width,height]);
    await chooseImage(page,{name:'shape.png',mimeType:'image/png',buffer:Buffer.from(encoded,'base64')});
    if ((await screen(page).textContent()).includes('Image fit: Fit')) await clickControl(page,'Image fit: Fit');
    await clickControl(page,'Adjust framing');
    await expect(screen(page)).toContainText('Preview · Zoom: 100%');
    await expect(page.locator('#background-preview-frame')).toHaveAttribute('data-editing','true');
    await expect.poll(()=>page.locator('#background-preview').evaluate(image=>parseFloat(image.style.width)/parseFloat(image.style.height))).toBeCloseTo(width/height,3);
    await page.screenshot({path:`/tmp/framing-${testInfo.project.name}-${width}x${height}.png`});
    await clickControl(page,'Done');await expect(screen(page)).toContainText('Background darkness:');
    await clickControl(page,'Apply');await expect(screen(page)).not.toContainText('Background darkness:');
    expect(await (await page.request.get('/visual/background')).body()).toEqual(Buffer.from(encoded,'base64'));
  }
});
