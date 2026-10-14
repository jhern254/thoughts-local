import {readFileSync} from 'node:fs';
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

function pngChunk(kind, payload) {
  const chunk = Buffer.alloc(payload.length + 12);
  chunk.writeUInt32BE(payload.length); chunk.write(kind, 4); payload.copy(chunk, 8);
  // PNG CRC-32 covers type+payload, starts/ends with all-bit inversion,
  // and uses reflected polynomial EDB88320 (IEEE CRC-32).
  let crc = 0xffffffff;
  for (const byte of chunk.subarray(4, -4)) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ ((crc & 1) ? 0xedb88320 : 0);
  }
  chunk.writeUInt32BE((crc ^ 0xffffffff) >>> 0, chunk.length - 4);
  return chunk;
}
// JPEG length counts its own two bytes, excluding the FF+marker prefix.
function jpegSegment(marker, payload) {
  const header = Buffer.from([255, marker, 0, 0]); header.writeUInt16BE(payload.length + 2, 2);
  return Buffer.concat([header, payload]);
}
function insertMetadata(source, format, metadata) {
  // Two-byte JPEG SOI, or eight-byte PNG signature plus 25-byte IHDR chunk.
  const offset = format === 'jpeg' ? 2 : 33;
  return Buffer.concat([source.subarray(0, offset), metadata, source.subarray(offset)]);
}
// Browser canvas encoders may add their own profile. Replace that declaration
// when constructing the synthetic wide-gamut fixture instead of duplicating it.
function withoutColorMetadata(source, format) {
  const parts = [source.subarray(0, format === 'jpeg' ? 2 : 8)];
  if (format === 'jpeg') {
    let offset = 2;
    // Canvas JPEG headers have length-bearing segments up to SOS (DA/218).
    // APP2 (E2/226) is removed so the fixture has one authoritative ICC profile.
    while (source[offset] === 255 && source[offset + 1] !== 218) {
      const end = offset + 2 + source.readUInt16BE(offset + 2);
      if (source[offset + 1] !== 226) parts.push(source.subarray(offset, end));
      offset = end;
    }
    parts.push(source.subarray(offset));
  } else {
    for (let offset = 8; offset < source.length;) {
      const end = offset + 12 + source.readUInt32BE(offset);
      if (!['iCCP','sRGB','gAMA','cHRM'].includes(source.toString('ascii', offset + 4, offset + 8))) parts.push(source.subarray(offset, end));
      offset = end;
    }
  }
  return Buffer.concat(parts);
}
async function displayedSamples(page, bytes) {
  return page.evaluate(async encoded => {
    const blob = new Blob([Uint8Array.from(atob(encoded), character => character.charCodeAt(0))]);
    const bitmap = await createImageBitmap(blob);
    const canvas = document.createElement('canvas'); canvas.width = bitmap.width; canvas.height = bitmap.height;
    const context = canvas.getContext('2d'); context.drawImage(bitmap, 0, 0); bitmap.close();
    const points = [[.2,.2], [.8,.2], [.2,.8], [.8,.8]];
    return {size: [canvas.width, canvas.height], pixels: points.map(([x,y]) =>
      [...context.getImageData(Math.floor(x * canvas.width), Math.floor(y * canvas.height), 1, 1).data])};
  }, bytes.toString('base64'));
}

test('stored pixels match display-oriented previews for all EXIF orientations', async ({page}, testInfo) => {
  test.setTimeout(90000);
  await page.goto('/'); await expect(page.locator('#status')).toHaveText('Connected');
  for (const format of ['jpeg', 'png']) {
    const source = Buffer.from(await page.evaluate(format => {
      const canvas = document.createElement('canvas'); canvas.width = 4096; canvas.height = 256;
      const context = canvas.getContext('2d');
      for (const [x,y,color] of [[0,0,'#458ad0'],[2048,0,'#c35b40'],[0,128,'#43b573'],[2048,128,'#ab69bd']]) {
        context.fillStyle = color; context.fillRect(x, y, 2048, 128);
      }
      return canvas.toDataURL(`image/${format}`, 1).split(',')[1];
    }, format), 'base64');
    for (let orientation = 1; orientation <= 8; orientation++) {
      // Classic little-endian TIFF: II, magic 42, IFD0 offset 8, one
      // Orientation (0112) SHORT entry, inline value, zero next-IFD pointer.
      const tiff = Buffer.from([73,73,42,0,8,0,0,0,1,0,18,1,3,0,1,0,0,0,orientation,0,0,0,0,0,0,0]);
      const privateMarker = Buffer.from('PRIVATE GPS camera serial timestamp');
      // JPEG APP1 (225) contains EXIF; COM (254) holds the synthetic private marker.
      const metadata = format === 'jpeg'
        ? Buffer.concat([jpegSegment(225, Buffer.concat([Buffer.from('Exif\0\0'), tiff, privateMarker])), jpegSegment(254, privateMarker)])
        : Buffer.concat([pngChunk('eXIf', Buffer.concat([tiff, privateMarker])), pngChunk('tEXt', Buffer.concat([Buffer.from('Comment\0'), privateMarker]))]);
      const buffer = insertMetadata(source, format, metadata);
      const expected = await displayedSamples(page, buffer);
      await open(page); await chooseImage(page, {name:`orientation-${orientation}.${format}`, mimeType:`image/${format}`, buffer});
      // Selection is acknowledged before queued preview placement paints.
      // Wait for this image's geometry rather than reading the previous frame.
      await expect.poll(() => page.locator('#background-preview').evaluate(image =>
        parseFloat(image.style.width) / parseFloat(image.style.height))).toBeCloseTo(expected.size[0] / expected.size[1], 4);
      await clickControl(page, 'Apply'); await expect(screen(page)).not.toContainText('Background darkness:');
      const response = await page.request.get('/visual/background'); const stored = await response.body();
      expect(stored.includes(privateMarker)).toBe(false);
      const actual = await displayedSamples(page, stored);
      // Transposed orientations swap axes; the 3072-axis budget shrinks 4096×256
      // to 3072×192 (or 192×3072). Color samples are inside uniform quadrants.
      expect(actual.size).toEqual(orientation >= 5 ? [192,3072] : [3072,192]);
      for (let point=0; point<4; point++) for (let channel=0; channel<4; channel++) {
        expect(Math.abs(actual.pixels[point][channel] - expected.pixels[point][channel])).toBeLessThanOrEqual(format === 'jpeg' ? 4 : 0);
      }
      if (orientation === 6) await page.screenshot({path:`/tmp/background-${testInfo.project.name}-${format}-normalized.png`});
    }
  }
});

test('sanitization preserves browser-managed wide-gamut color and removes private profile tags', async ({page}, testInfo) => {
  await page.goto('/'); await expect(page.locator('#status')).toHaveText('Connected');
  const profile = readFileSync(new URL('./fixtures/visual-matrix-rgb.icc', import.meta.url));
  for (const format of ['jpeg','png']) {
    const source = Buffer.from(await page.evaluate(format => {
      const canvas = document.createElement('canvas'); canvas.width=1200; canvas.height=850;
      const context=canvas.getContext('2d');
      for (const [x,y,color] of [[0,0,'#689879'],[600,0,'#9a6265'],[0,425,'#536e9c'],[600,425,'#96783f']]) {
        context.fillStyle=color; context.fillRect(x,y,600,425);
      }
      return canvas.toDataURL(`image/${format}`, 1).split(',')[1];
    }, format), 'base64');
    let metadata;
    if (format === 'jpeg') metadata = jpegSegment(226, Buffer.concat([Buffer.from('ICC_PROFILE\0'), Buffer.from([1,1]), profile]));
    else {
      const {deflateSync} = await import('node:zlib');
      metadata = pngChunk('iCCP', Buffer.concat([Buffer.from('Synthetic private profile\0\0'), deflateSync(profile)]));
    }
    const buffer = insertMetadata(withoutColorMetadata(source, format), format, metadata);
    const intended = await displayedSamples(page, buffer);
    const untagged = await displayedSamples(page, source);
    // Establish that this is a color-management test, not inert byte copying.
    expect(Math.max(...intended.pixels.flatMap((pixel,i) => pixel.map((value,c) => Math.abs(value - untagged.pixels[i][c]))))).toBeGreaterThan(8);
    await open(page); await chooseImage(page, {name:`wide-gamut.${format}`, mimeType:`image/${format}`, buffer});
    await clickControl(page, 'Apply'); await expect(screen(page)).not.toContainText('Background darkness:');
    const response = await page.request.get('/visual/background'); const stored = await response.body();
    const actual = await displayedSamples(page, stored);
    for (let point=0; point<4; point++) for (let channel=0; channel<4; channel++) {
      expect(Math.abs(actual.pixels[point][channel] - intended.pixels[point][channel])).toBeLessThanOrEqual(format === 'jpeg' ? 4 : 0);
    }
    await page.screenshot({path:`/tmp/background-${testInfo.project.name}-${format}-profile.png`});
    await page.keyboard.press('ArrowLeft'); await page.keyboard.press('ArrowRight');
  }
});
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
  let backgroundReads = 0;
  page.on('request', request => {
    if (request.method() === 'GET' && request.url().endsWith('/visual/background')) backgroundReads++;
  });
  await page.evaluate(() => { window.savedBackgroundElement = document.getElementById('background-image'); });
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
  expect(backgroundReads).toBe(0);
  expect(await page.evaluate(() => document.getElementById('background-image') === window.savedBackgroundElement)).toBe(true);
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
test('publishes a decoded odd-sized wallpaper before returning and moving entity selection', async ({page}, testInfo) => {
  await page.addInitScript(() => {
    // A staged image can decode before a separate visible element loads its URL.
    // Hold that handoff across a paint so readiness doesn't depend on a warm cache.
    const source = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, 'src');
    Object.defineProperty(HTMLImageElement.prototype, 'src', {
      ...source,
      set(value) {
        if (this.id === 'background-image') {
          setTimeout(() => source.set.call(this, value), 50);
        } else {
          source.set.call(this, value);
        }
      }
    });
    const decode = HTMLImageElement.prototype.decode;
    const decodedImages = new WeakSet();
    HTMLImageElement.prototype.decode = async function (...args) {
      await decode.apply(this, args);
      decodedImages.add(this);
    };
    // Inspect publication before a paint or later load event can hide an unready image.
    window.backgroundPublications = [];
    new MutationObserver(() => {
      const surface = document.getElementById('background-surface');
      const image = document.getElementById('background-image');
      if (!surface || surface.hidden || !image || image.hidden) return;
      window.backgroundPublications.push({
        decoded: decodedImages.has(image),
        naturalWidth: image.naturalWidth, naturalHeight: image.naturalHeight,
        width: parseFloat(image.style.width), height: parseFloat(image.style.height),
        left: parseFloat(image.style.left), top: parseFloat(image.style.top),
        viewportWidth: innerWidth, viewportHeight: innerHeight
      });
    }).observe(document, {subtree: true, childList: true, attributes: true, attributeFilter: ['hidden', 'src', 'style']});
  });
  let sockets = 0;
  page.on('websocket', () => sockets++);
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await open(page);
  await clickControl(page, 'Remove background');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await page.keyboard.press('Tab');
  await expect(screen(page)).toContainText('Tab/Q: events');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Background darkness:');
  await expect(screen(page)).not.toContainText('Loading visual');
  for (const size of [{width:91, height:4093}, {width:4093, height:91}]) {
    const encoded = await page.evaluate(({width, height}) => {
      const canvas = document.createElement('canvas'); canvas.width=width; canvas.height=height;
      const context = canvas.getContext('2d');
      context.fillStyle='#64a0d0'; context.fillRect(0, 0, width, height);
      return canvas.toDataURL('image/png').split(',')[1];
    }, size);
    await chooseImage(page, {name:'odd-size.png', mimeType:'image/png', buffer:Buffer.from(encoded, 'base64')});
    if ((await screen(page).textContent()).includes('Background fit: Fit')) await clickControl(page, 'Background fit: Fit');
    await darkness(page, 30);
    await page.evaluate(() => { window.backgroundPublications = []; });
    await clickControl(page, 'Apply');
    await expect(screen(page)).not.toContainText('Background darkness:');
    await expect(page.locator('#background-image')).toBeVisible();
    await page.keyboard.press('ArrowLeft');
    await expect(page.locator('.xterm-bg-62').filter({hasText:/^Subjects$/})).toBeVisible();
    await page.keyboard.press('ArrowRight');
    await expect(page.locator('.xterm-bg-62').filter({hasText:/^Options$/})).toBeVisible();
    const publications = await page.evaluate(() => window.backgroundPublications);
    expect(publications.length).toBeGreaterThan(0);
    for (const frame of publications) {
      expect(frame.naturalWidth).toBeGreaterThan(0);
      expect(frame.naturalHeight).toBeGreaterThan(0);
      expect(frame.width).toBeGreaterThan(0);
      expect(frame.height).toBeGreaterThan(0);
      expect(frame.left).toBeLessThanOrEqual(0);
      expect(frame.top).toBeLessThanOrEqual(0);
      expect(frame.left + frame.width).toBeGreaterThanOrEqual(frame.viewportWidth);
      expect(frame.top + frame.height).toBeGreaterThanOrEqual(frame.viewportHeight);
      expect(frame.decoded).toBe(true);
    }
    await expect(page.locator('#background-preview-wrap')).toBeHidden();
    await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
    await page.screenshot({path:`/tmp/upload-return-${testInfo.project.name}-${size.width}x${size.height}.png`});
    await page.keyboard.press('Enter');
    await expect(screen(page)).toContainText('Background darkness:');
    await expect(screen(page)).not.toContainText('Loading visual');
  }
  await clickControl(page, 'Remove background');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await expect(page.locator('#background-image')).toBeHidden();
  expect(sockets).toBe(1);
});
test('failed replacement retains the active image', async ({page}) => {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await open(page);
  await chooseImage(page, await imageFixture(page));
  await clickControl(page, 'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
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
  await clickControl(page, 'Choose image or video');
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
  if ((await screen(page).textContent()).includes('Background fit: Fit')) await clickControl(page,'Background fit: Fit');
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
  await clickControl(page,'Background fit: Fill');
  await expect(screen(page)).toContainText('Background fit: Fit');
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
  for (const [width,height] of [[500,500],[4096,256],[4,4096]]) {
    await open(page);
    const encoded=await page.evaluate(([width,height])=>{
      const canvas=document.createElement('canvas');canvas.width=width;canvas.height=height;
      const context=canvas.getContext('2d');
      context.fillStyle='#3388aa';context.fillRect(0,0,width,height);
      context.fillStyle='#ddbb44';context.fillRect(0,0,width/2,height/2);
      return canvas.toDataURL('image/png').split(',')[1];
    },[width,height]);
    await chooseImage(page,{name:'shape.png',mimeType:'image/png',buffer:Buffer.from(encoded,'base64')});
    if ((await screen(page).textContent()).includes('Background fit: Fit')) await clickControl(page,'Background fit: Fit');
    await clickControl(page,'Adjust framing');
    await expect(screen(page)).toContainText('Preview · Zoom: 100%');
    await expect(page.locator('#background-preview-frame')).toHaveAttribute('data-editing','true');
    await expect.poll(()=>page.locator('#background-preview').evaluate(image=>parseFloat(image.style.width)/parseFloat(image.style.height))).toBeCloseTo(width/height,3);
    if (width===4) {
      const colors=await page.locator('#background-preview').evaluate(image=>{
        const canvas=document.createElement('canvas');canvas.width=image.naturalWidth;canvas.height=image.naturalHeight;
        const context=canvas.getContext('2d');context.drawImage(image,0,0);
        return [Array.from(context.getImageData(0,0,1,1).data),Array.from(context.getImageData(canvas.width-1,0,1,1).data)];
      });
      expect(colors).toEqual([[221,187,68,255],[51,136,170,255]]);
    }
    await page.screenshot({path:`/tmp/framing-${testInfo.project.name}-${width}x${height}.png`});
    await clickControl(page,'Done');await expect(screen(page)).toContainText('Background darkness:');
    await clickControl(page,'Apply');await expect(screen(page)).not.toContainText('Background darkness:');
    if (width === 500) {
      const stored = await (await page.request.get('/visual/background')).body();
      expect(await displayedSamples(page, stored)).toEqual(await displayedSamples(page, Buffer.from(encoded,'base64')));
    } else {
      const size = await page.locator('#background-image').evaluate(image => [image.naturalWidth,image.naturalHeight]);
      expect(size).toEqual(width === 4096 ? [3072,192] : [3,3072]);
    }
  }
});
