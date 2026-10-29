import {test, expect} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {mkdtempSync, readFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {openVisual, clickControl, screen} from './visual-helpers.mjs';

let source, longSource, offsetSource, fragmentedSource, matroskaSource, singleVideoMatroska;
// The native settings outlive each browser context. Give motion assertions an
// explicit user preference rather than inheriting the preceding test's pause.
test.beforeEach(async({request,baseURL})=>{
  const response=await request.put('/visual/settings',{headers:{Origin:new URL(baseURL).origin},data:{darkness:70,framing:{fit:'fill',zoom:100,positionX:5000,positionY:5000},speedPercent:100,userPaused:false}});
  expect(response.ok()).toBe(true);
});
test.beforeAll(() => {
  const directory = mkdtempSync(join(tmpdir(),'thoughts-trim-fixture-'));
  try {
    const path = join(directory,'source.mp4');
    execFileSync('ffmpeg',['-v','error','-f','lavfi','-i','testsrc2=size=96x64:rate=24:duration=60','-c:v','libx264','-threads','1','-g','120','-pix_fmt','yuv420p','-movflags','+faststart',path]);
    source = {name:'synthetic-minute.mp4',mimeType:'video/mp4',buffer:readFileSync(path)};
    const fragmentedPath=join(directory,'fragmented.mp4');
    execFileSync('ffmpeg',['-v','error','-i',path,'-c','copy','-movflags','+frag_keyframe+empty_moov+default_base_moof+global_sidx',fragmentedPath]);
    fragmentedSource={name:'fragmented.mp4',mimeType:'video/mp4',buffer:readFileSync(fragmentedPath)};
    const singleVideoPath=join(directory,'single-video.mkv');
    execFileSync('ffmpeg',['-v','error','-i',path,'-map','0:v:0','-c','copy',singleVideoPath]);
    singleVideoMatroska={name:'single-video.mkv',mimeType:'video/x-matroska',buffer:readFileSync(singleVideoPath)};
    const longPath=join(directory,'four-hours.mp4');
    execFileSync('ffmpeg',['-v','error','-f','lavfi','-i','testsrc2=size=32x24:rate=1:duration=14400','-c:v','libx264','-threads','1','-g','60','-pix_fmt','yuv420p','-movflags','+faststart',longPath]);
    longSource={name:'four-hours.mp4',mimeType:'video/mp4',buffer:readFileSync(longPath)};
    const mkvPath=join(directory,'movie.mkv');
    execFileSync('ffmpeg',['-v','error','-f','lavfi','-i',"nullsrc=size=160x90:rate=24:duration=12,geq=lum='32+10*T':cb=128:cr=128",'-f','lavfi','-i','sine=duration=12','-map','0:v','-map','1:a','-map','1:a','-map','1:a','-map','1:a','-c:v','libx264','-threads','1','-g','120','-pix_fmt','yuv420p','-c:a','aac',mkvPath]);
    matroskaSource={name:'synthetic-movie.mkv',mimeType:'video/x-matroska',buffer:readFileSync(mkvPath)};
    const offsetPath=join(directory,'offset.mp4');
    execFileSync('ffmpeg',['-v','error','-f','lavfi','-i',"nullsrc=size=64x48:rate=24:duration=12,geq=lum='32+12*T':cb=128:cr=128",'-c:v','libx264','-threads','1','-pix_fmt','yuv420p','-output_ts_offset','5','-movflags','+faststart',offsetPath]);
    offsetSource={name:'offset.mp4',mimeType:'video/mp4',buffer:readFileSync(offsetPath)};
  } finally {rmSync(directory,{recursive:true,force:true});}
});

async function stageSource(page) {
  const chooser = page.waitForEvent('filechooser');
  await clickControl(page,'Choose image or video');
  await (await chooser).setFiles(source);
  await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
  await expect(page.locator('#background-preview-wrap')).toBeVisible();
}

for (const savedBackground of [false, true]) test(`source stays visible during scrubbing with saved background ${savedBackground}`, async ({page, request, baseURL}, testInfo) => {
  let sourceUploads = 0, preparations = 0, sockets = 0;
  page.on('websocket', () => sockets++);
  page.on('request', request => {
    if (request.method() !== 'POST') return;
    const pathname = new URL(request.url()).pathname;
    if (pathname === '/visual/video/source') sourceUploads++;
    if (pathname === '/visual/video/preview') preparations++;
  });
  const headers = {Origin: new URL(baseURL).origin};
  expect((await request.delete('/visual/background', {headers})).ok()).toBe(true);
  if (savedBackground) {
    const image = await page.evaluate(() => {
      const canvas = document.createElement('canvas');
      canvas.width = canvas.height = 16;
      const context = canvas.getContext('2d');
      context.fillStyle = '#ff00ff';
      context.fillRect(0, 0, 16, 16);
      return canvas.toDataURL('image/png').split(',')[1];
    });
    expect((await request.post('/visual/background', {headers, multipart: {
      image: {name:'synthetic.png', mimeType:'image/png', buffer:Buffer.from(image, 'base64')},
      darkness:'70', framing:JSON.stringify({fit:'fill', zoom:100, positionX:5000, positionY:5000})
    }})).ok()).toBe(true);
  }
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await openVisual(page);
  await stageSource(page);
  const connectedSockets = sockets;
  const sourceVideo = page.locator('#background-preview-video');
  const fallback = page.locator('#background-preview');
  await expect(sourceVideo).toBeVisible();
  await expect(fallback).toBeHidden();
  await sourceVideo.evaluate(video => {
    window.scrubSourceVideo = video;
    window.sourceSeekVisibility = [];
    const recordVisibility = () => window.sourceSeekVisibility.push({
      sourceVisible: !video.hidden && getComputedStyle(video).display !== 'none',
      fallbackHidden: getComputedStyle(document.getElementById('background-preview')).display === 'none'
    });
    video.addEventListener('seeking', recordVisibility);
    video.addEventListener('seeked', recordVisibility);
  });
  await page.keyboard.down('ArrowRight');
  for (let repeat = 0; repeat < 12; repeat++) await page.keyboard.down('ArrowRight');
  await page.keyboard.up('ArrowRight');
  await expect(screen(page)).toContainText('Source / Start: 00:00:01.300');
  await expect.poll(() => sourceVideo.evaluate(video => video.seeking ? -1 : video.currentTime)).toBeCloseTo(1.3, 3);
  await page.keyboard.press('PageDown');
  await page.keyboard.press('PageUp');
  await page.keyboard.press('PageDown');
  await page.keyboard.press('Shift+ArrowLeft');
  await page.keyboard.press('ArrowRight');
  await expect(screen(page)).toContainText('Source / Start: 00:00:58.100');
  await expect.poll(() => sourceVideo.evaluate(video => video.seeking ? -1 : video.currentTime)).toBeCloseTo(58.1, 3);
  expect(sourceUploads).toBe(1);
  expect(preparations).toBe(0);
  expect(sockets).toBe(connectedSockets);
  expect(await sourceVideo.evaluate(video => video === window.scrubSourceVideo)).toBe(true);
  const visibility = await page.evaluate(() => window.sourceSeekVisibility);
  expect(visibility.length).toBeGreaterThanOrEqual(2);
  expect(visibility.every(sample => sample.sourceVisible && sample.fallbackHidden)).toBe(true);
  await page.screenshot({path:`/tmp/seek-visible-${testInfo.project.name}-${savedBackground}.png`});
  await clickControl(page, 'Cancel upload');
  await expect(screen(page)).toContainText('Background darkness:');
  if (savedBackground) await expect(fallback).toBeVisible();
});

test('End previews backward and held arrows accelerate without uploading or preparing again', async ({page}, testInfo) => {
  let uploads = 0, preparations = 0;
  page.on('request', request => {
    if (request.method() !== 'POST') return;
    const pathname = new URL(request.url()).pathname;
    if (pathname === '/visual/video/source') uploads++;
    if (pathname === '/visual/video/preview') preparations++;
  });
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await openVisual(page);
  await stageSource(page);
  const sourceVideo = page.locator('#background-preview-video');
  await clickControl(page, 'End: 00:00:30.000 (max 00:00:30.000)');
  await page.keyboard.press('ArrowLeft');
  await expect(screen(page)).toContainText('End: 00:00:29.900');
  await expect(screen(page)).toContainText('Source / Start: 00:00:00.000');
  await expect.poll(() => sourceVideo.evaluate(video => video.currentTime)).toBeCloseTo(29.899, 3);
  await page.keyboard.press('Shift+ArrowLeft');
  await expect.poll(() => sourceVideo.evaluate(video => video.currentTime)).toBeCloseTo(28.899, 3);
  await expect(sourceVideo).toBeVisible();
  await page.screenshot({path: `/tmp/trim-end-${testInfo.project.name}.png`});

  await clickControl(page, 'Source / Start: 00:00:00.000 / 00:01:00.000');
  await page.keyboard.down('ArrowRight');
  await expect(screen(page)).toContainText('Source / Start: 00:00:00.100');
  for (let repeat = 0; repeat < 19; repeat++) await page.keyboard.down('ArrowRight');
  await expect(screen(page)).toContainText('Source / Start: 00:00:02.000');
  for (let repeat = 19; repeat < 36; repeat++) await page.keyboard.down('ArrowRight');
  await expect(screen(page)).toContainText('Source / Start: 00:00:23.000');
  await page.keyboard.down('ArrowRight');
  await expect(screen(page)).toContainText('Source / Start: 00:00:28.000');
  await page.keyboard.up('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await expect(screen(page)).toContainText('Source / Start: 00:00:28.100');
  await expect.poll(() => sourceVideo.evaluate(video => video.currentTime)).toBeCloseTo(28.1, 3);
  await expect(sourceVideo).toBeVisible();
  expect(uploads).toBe(1);
  expect(preparations).toBe(0);
  await page.setViewportSize({width:650, height:750});
  await expect(screen(page)).toContainText('Hold: faster');
  await page.screenshot({path: `/tmp/trim-scrub-${testInfo.project.name}-narrow.png`});
});

test('preparing a loop reserves a braille wave above Stop rendering and keeps keyboard cancellation available',async({page},testInfo)=>{
  let sockets=0, busyAdmissions=0, releasePreparation;
  page.on('websocket',socket=>{
    sockets++;
    socket.on('framereceived',({payload})=>{
      const frame=Buffer.from(payload);
      if(frame[0]===55&&frame.subarray(1).toString()==='busy')busyAdmissions++;
    });
  });
  // Admission may retry while a previous tab retires. Exercise that boundary,
  // then require exactly one accepted connection and no new sockets in the editor.
  const retiring=await page.context().newPage();
  await retiring.goto('/');await expect(retiring.locator('#status')).toHaveText('Connected');
  await page.goto('/');
  await expect.poll(()=>busyAdmissions).toBeGreaterThan(0);
  await retiring.close();
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Events');
  const acceptedSockets=sockets;
  expect(acceptedSockets-busyAdmissions).toBe(1);
  await openVisual(page);await stageSource(page);
  await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');
  await page.keyboard.press('Home');
  await expect(screen(page)).toContainText('End: 00:00:01.000');
  const done = screen(page).getByText('Done — use previewed loop', {exact:true});
  await expect(done).toHaveCSS('color', 'rgb(153, 153, 153)');
  await clickControl(page, 'Done — use previewed loop');
  await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
  const previewControl = screen(page).getByText('Preview loop', {exact:true});
  const previewBefore = await previewControl.boundingBox();
  const terminalBefore = await page.locator('.xterm-screen').boundingBox();
  await page.screenshot({path:`/tmp/video-trim-idle-${testInfo.project.name}.png`});
  const gate=new Promise(resolve=>{releasePreparation=resolve;});
  await page.route('**/visual/video/preview',async route=>{
    if(route.request().method()==='POST')await gate;
    await route.continue();
  });
  try {
    await clickControl(page,'Preview loop');
    const bar=page.locator('#video-preparation-wave');
    await expect(page.getByRole('progressbar')).toBeVisible();
    await expect.poll(async () => (await screen(page).getByText('Stop rendering', {exact:true}).boundingBox())?.y).toBe(previewBefore.y);
    expect(await page.locator('.xterm-screen').boundingBox()).toEqual(terminalBefore);
    await expect(done).toHaveCSS('color','rgb(153, 153, 153)');
    await clickControl(page,'Done — use previewed loop');
    await expect(screen(page)).toContainText('Stop rendering');
    await expect(bar).not.toHaveAttribute('aria-valuenow');
    const lightPosition=()=>bar.locator('.preparation-sweep').first().evaluate(element=>getComputedStyle(element).transform);
    const position=await lightPosition();
    await expect.poll(lightPosition).not.toBe(position);
    await expect(bar.locator('pre')).toHaveCount(2);
    async function expectWavePlacement() {
      await expect.poll(()=>page.evaluate(()=>{
        const indicator=document.getElementById('video-preparation').getBoundingClientRect();
        const expectedHeight=Math.min(112,Math.max(72,innerHeight*0.14))-8;
        const wave=document.getElementById('video-preparation-wave').getBoundingClientRect();
        const preview=document.getElementById('background-preview-wrap').getBoundingClientRect();
        const rows=[...document.querySelectorAll('.xterm-rows > div')];
        const speedRow=rows.find(row=>row.textContent.includes('Playback speed:'));
        const previewRow=rows.find(row=>row.textContent.trim()==='Stop rendering');
        if(!previewRow)return false;
        const speed=speedRow?.getBoundingClientRect();
        const action=previewRow.getBoundingClientRect();
        const aligned=!preview.width || (Math.abs(wave.left-preview.left)<1 && Math.abs(wave.width-preview.width)<1);
        return indicator.height>20 && indicator.height<=expectedHeight+1 && (!speed || wave.top>=speed.bottom) && wave.top>=preview.bottom && wave.bottom<=action.top+1 && aligned;
      })).toBe(true);
    }
    await expectWavePlacement();
    expect(await page.locator('#video-preparation').evaluate(indicator => {
      const label = indicator.querySelector('[role="status"]').getBoundingClientRect();
      indicator.style.pointerEvents = 'auto';
      const visibleAboveTerminal = indicator.contains(document.elementFromPoint(label.x + 2, label.y + 2));
      indicator.style.pointerEvents = '';
      return visibleAboveTerminal;
    })).toBe(true);
    await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
    await page.screenshot({path:`/tmp/video-loading-${testInfo.project.name}-desktop.png`});
    await page.setViewportSize({width:650,height:600});
    await expect(bar).toBeInViewport();
    await expectWavePlacement();
    await page.emulateMedia({reducedMotion:'reduce'});
    await page.waitForTimeout(100);
    const still=await lightPosition();
    await page.waitForTimeout(300);
    expect(await lightPosition()).toBe(still);
    await page.screenshot({path:`/tmp/video-loading-${testInfo.project.name}-narrow.png`});
    await page.emulateMedia({reducedMotion:'no-preference'});
    await expect.poll(lightPosition).not.toBe(still);
    await page.evaluate(()=>{
      Object.defineProperty(document,'hidden',{configurable:true,get:()=>true});
      document.dispatchEvent(new Event('visibilitychange'));
    });
    const hiddenFrame=await lightPosition();
    await page.waitForTimeout(300);
    expect(await lightPosition()).toBe(hiddenFrame);
    await page.evaluate(()=>{
      delete document.hidden;
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await expect.poll(lightPosition).not.toBe(hiddenFrame);
    // Inspect all action styles at full size; compact scrolling is covered separately.
    await page.setViewportSize({width:1200,height:850});
    await expect.poll(async()=>(await screen(page).getByText('Stop rendering',{exact:true}).boundingBox())?.y).toBe(previewBefore.y);
    const previewPending = await screen(page).getByText('Stop rendering', {exact:true}).boundingBox();
    releasePreparation();
    await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
    await expect.poll(async () => (await previewControl.boundingBox())?.y).toBe(previewPending.y);
    await expect(done).not.toHaveCSS('color', 'rgb(153, 153, 153)');
    await expect(previewControl).toHaveCSS('color', 'rgb(153, 153, 153)');
    await page.screenshot({path:`/tmp/video-trim-ready-${testInfo.project.name}.png`});
    await expect(bar).toBeHidden();
    expect(await bar.evaluate(element=>element.getAnimations({subtree:true}).length)).toBe(0);
    const retiredFrame=await lightPosition();
    await page.waitForTimeout(300);
    expect(await lightPosition()).toBe(retiredFrame);
    await page.unroute('**/visual/video/preview');

    await clickControl(page, 'Preview loop');
    await page.keyboard.press('Shift+Tab');
    await expect.poll(async () => (await page.locator('.xterm-bg-62').allTextContents()).join('')).toContain('Playback speed:');
    await page.keyboard.press('ArrowLeft');
    await expect(screen(page)).toContainText('Playback speed: 95%');
    await expect(previewControl).toHaveCSS('color', 'rgb(153, 153, 153)');
    await clickControl(page, 'End: 00:00:01.000 (max 00:00:30.000)');
    await page.keyboard.press('ArrowRight');
    await expect(previewControl).not.toHaveCSS('color', 'rgb(153, 153, 153)');
    await page.keyboard.press('ArrowLeft');
    await expect(previewControl).toHaveCSS('color', 'rgb(153, 153, 153)');
    await page.keyboard.press('ArrowRight');
    await expect(previewControl).not.toHaveCSS('color', 'rgb(153, 153, 153)');

    let releaseFailure;
    const failureGate=new Promise(resolve=>{releaseFailure=resolve;});
    await page.route('**/visual/video/preview',async route=>{
      if(route.request().method()==='POST'){
        await failureGate;
        await route.fulfill({status:500,body:'Could not prepare loop.'});
      } else await route.continue();
    });
    try {
      await clickControl(page,'Preview loop');
      await expect(bar).toBeVisible();
      releaseFailure();
      await expect(screen(page)).toContainText('Could not prepare this loop.');
      await expect(bar).toBeHidden();
      await expect(screen(page)).toContainText('Range: 00:00:00 - 00:00:01');
    } finally { releaseFailure(); }
    await page.unroute('**/visual/video/preview');

    let releaseCancelled;
    const cancelGate=new Promise(resolve=>{releaseCancelled=resolve;});
    await page.route('**/visual/video/preview',async route=>{
      if(route.request().method()==='POST')await cancelGate;
      await route.continue();
    });
    try {
      await clickControl(page,'Preview loop');
      await expect(bar).toBeVisible();
      await page.keyboard.press('Tab');
      await expect.poll(async()=>(await page.locator('.xterm-bg-62').allTextContents()).map(text=>text.trim())).toContain('Cancel upload');
      await page.keyboard.press('Enter');
      await expect(bar).toBeHidden();
      await expect(screen(page)).toContainText('Background darkness:');
      expect(sockets).toBe(acceptedSockets);
      expect(sockets-busyAdmissions).toBe(1);
    } finally { releaseCancelled(); }
  } finally { releasePreparation(); await page.unrouteAll({behavior:'wait'}); }
});

test('repeated cancellation keeps one discard owner until Visual is restored',async({page},testInfo)=>{
  let sockets=0, deletes=0, latestOptions, releasePreparation, releaseDiscard;
  const discardIDs=new Set();
  page.on('websocket',socket=>{
    sockets++;
    socket.on('framereceived',({payload})=>{
      const frame=Buffer.from(payload);
      if(frame[0]!==111)return;
      latestOptions=JSON.parse(frame.subarray(1).toString());
      if(latestOptions.request==='discardVideo')discardIDs.add(latestOptions.id);
    });
  });
  await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
  await openVisual(page);await stageSource(page);
  const preparationGate=new Promise(resolve=>{releasePreparation=resolve;});
  const discardGate=new Promise(resolve=>{releaseDiscard=resolve;});
  await page.route('**/visual/video/preview',async route=>{
    if(route.request().method()==='POST')await preparationGate;
    await route.continue();
  });
  await page.route('**/visual/video/source',async route=>{
    if(route.request().method()==='DELETE'){deletes++;await discardGate;}
    await route.continue();
  });
  try {
    await clickControl(page,'Preview loop');
    await expect.poll(()=>latestOptions?.request).toBe('prepareLoop');
    const preparationID=latestOptions.id;
    await clickControl(page,'Cancel upload');
    await expect.poll(()=>latestOptions?.request).toBe('discardVideo');
    const discardID=latestOptions.id;
    expect(discardID).toBe(preparationID+1);
    await expect.poll(()=>deletes).toBe(1);
    await expect(screen(page)).toContainText('Cancelling upload…');
    for(const key of ['Escape','Escape','q','q','Enter'])await page.keyboard.press(key);
    await page.setViewportSize({width:650,height:600});
    await clickControl(page,'Cancel upload');await clickControl(page,'Cancel upload');
    expect(latestOptions.id).toBe(discardID);
    releaseDiscard();
    await expect(screen(page)).toContainText('Background darkness:');
    expect(deletes).toBe(1);
    expect([...discardIDs]).toEqual([discardID]);
    await expect(screen(page)).not.toContainText('Cancel upload');
    await expect(page.locator('#video-preparation')).toBeHidden();
    await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
    await page.screenshot({path:`/tmp/video-cancellation-${testInfo.project.name}.png`});
    await clickControl(page,'Background darkness: 70%');
    await page.keyboard.press('ArrowRight');
    await expect(screen(page)).toContainText('Background darkness: 71%');
    expect(sockets).toBe(1);
  } finally {
    releasePreparation();releaseDiscard();
    await page.unrouteAll({behavior:'wait'});
  }
});

test('late cuts preview exact encoded bytes; failed and unpreviewed edits keep the accepted loop',async({page},testInfo)=>{
  let sockets=0, prepares=0, applies=0;
  page.on('websocket',()=>sockets++);
  page.on('request',request=>{
    const url=new URL(request.url());
    if(url.pathname==='/visual/video/preview'&&request.method()==='POST')prepares++;
    if(url.pathname==='/visual/video'&&request.method()==='POST')applies++;
  });
  await page.addInitScript(()=>{
    // Hold the scrolled terminal paint briefly after native focus has moved.
    // This reproduces a slow CI render without delaying or retrying the click.
    const NativeSocket=window.WebSocket;
    window.WebSocket=class extends NativeSocket {
      set onmessage(callback) {
        super.onmessage=event=>{
          if(this.holdScroll && new Uint8Array(event.data)[0]===49) this.scrollPaints.push(()=>callback(event));
          else callback(event);
        };
      }
      send(data) {
        if(window.delayTrimScroll && new TextDecoder().decode(data)==='0\x1b[Z') {
          window.delayTrimScroll=false;this.holdScroll=true;this.scrollPaints=[];
          setTimeout(()=>{this.holdScroll=false;for(const paint of this.scrollPaints)paint();this.scrollPaints=[];},200);
        }
        return super.send(data);
      }
    };

    const nativeFetch=window.fetch;
    window.fetch=async(...args)=>{
      const response=await nativeFetch(...args);
      if(String(args[0]).includes('/visual/video/preview?')&&String(args[0]).endsWith('part=video'))
        window.trimPreviewBytes=[...new Uint8Array(await response.clone().arrayBuffer())];
      return response;
    };
  });
  await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
  const connectedSockets=sockets;
  await page.keyboard.press('t');await page.keyboard.insertText('Keep this trim draft 界');
  await openVisual(page);await stageSource(page);
  await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');
  await page.keyboard.press('End');
  await page.keyboard.press('Shift+ArrowRight');
  await expect(screen(page)).toContainText('End: 00:00:30.000 (max 00:00:30.000)');
  await clickControl(page,'Source / Start: 00:00:00.000 / 00:01:00.000');
  await page.keyboard.press('End');
  await expect(screen(page)).toContainText('Source / Start: 00:00:59.000 / 00:01:00.000');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Control+u');await page.keyboard.insertText('00:00:57.000');await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('End: 00:01:00.000 (max 00:01:00.000)');
  await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.currentTime)).toBe(57);
  await clickControl(page,'Preview loop');
  await expect(screen(page)).toContainText('Range: 00:00:57 - 00:01:00');
  await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.paused)).toBe(false);
  expect(await page.locator('#background-preview-video').evaluate(video=>video.duration)).toBe(3);
  // Capture the actual fetched bytes before they become a media blob.
  // CSP deliberately permits blob playback, not arbitrary blob fetch requests.
  const previewBytes=Buffer.from(await page.evaluate(()=>window.trimPreviewBytes));
  await page.screenshot({path:`/tmp/trim-${testInfo.project.name}-desktop.png`});
  await clickControl(page,'Source / Start: 00:00:57.000 / 00:01:00.000');await page.keyboard.press('Home');
  await expect(screen(page)).toContainText('End: 00:00:30.000 (max 00:00:30.000)');
  await expect(screen(page)).toContainText('Range: 00:00:57 - 00:01:00');
  expect(prepares).toBe(1);
  await page.keyboard.press('Enter');await page.keyboard.press('Control+u');await page.keyboard.insertText('00:00:57.000');await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('End: 00:01:00.000 (max 00:01:00.000)');
  // A failed replacement preserves the previously acknowledged loop.
  await page.keyboard.press('ArrowRight');
  await expect(screen(page)).toContainText('Source / Start: 00:00:57.100 / 00:01:00.000');
  expect(prepares).toBe(1);
  await page.route('**/visual/video/preview',async route=>{
    if(route.request().method()==='POST')await route.fulfill({status:500,body:'Could not prepare loop.'});else await route.continue();
  });
  await clickControl(page,'Preview loop');
  await expect(screen(page)).toContainText('Could not prepare this loop.');
  await expect(screen(page)).toContainText('Range: 00:00:57 - 00:01:00');
  expect(await page.locator('#background-preview-video').evaluate(video=>video.duration)).toBe(3);
  await page.unroute('**/visual/video/preview');
  await page.setViewportSize({width:650,height:750});
  await expect(screen(page)).toContainText('Done — use previewed loop');
  // The old view already contains Done. Wait for the resized native frame and
  // browser preview to agree before using terminal mouse coordinates.
  await expect.poll(()=>page.locator('#background-preview-wrap').evaluate(element=>{
    const preview=element.getBoundingClientRect();
    const rows=[...document.querySelectorAll('.xterm-rows > div')];
    const top=rows.find(row=>row.textContent.includes('╭'))?.getBoundingClientRect();
    const bottom=rows.find(row=>row.textContent.includes('╰'))?.getBoundingClientRect();
    return !element.hidden && preview.left>=0 && preview.right<=innerWidth &&
      preview.top>=0 && preview.bottom<=innerHeight &&
      Math.abs(preview.top-top?.bottom)<1 && Math.abs(preview.bottom-bottom?.top)<1;
  })).toBe(true);
  await page.screenshot({path:`/tmp/trim-${testInfo.project.name}-narrow.png`});
  await clickControl(page,'Done — use previewed loop');
  await expect(screen(page)).toContainText('Background darkness:');
  await expect.poll(()=>page.locator('.xterm-rows > div').evaluateAll(rows=>{
    const previewedRow=rows.findIndex(row=>row.textContent.trim().startsWith('Previewed:'));
    return previewedRow>=2 && rows[previewedRow-1].textContent.trim()==='' && rows[previewedRow-2].textContent.trim()==='Back';
  })).toBe(true);
  await page.screenshot({path:`/tmp/visual-spacing-${testInfo.project.name}.png`});
  await clickControl(page,'Apply');
  await expect(screen(page)).toContainText('Keep this trim draft 界');
  expect(applies).toBe(1);expect(prepares).toBe(2);expect(sockets).toBe(connectedSockets);
  const stored=await page.request.get('/visual/background');
  expect(stored.ok()).toBe(true);
  expect(Buffer.compare(await stored.body(),previewBytes)).toBe(0);
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(false);
  // A new upload exclusively owns playback while editing; Cancel restores the
  // already selected video without another session or persistent pause change.
  await openVisual(page);await stageSource(page);
  await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(true);
  // At the narrow viewport Cancel can be below the control window. Move back
  // from Source to reveal it, as a keyboard user would, before measuring a click.
  await page.evaluate(()=>{window.delayTrimScroll=true;});
  await page.keyboard.press('Shift+Tab');
  await expect(screen(page).getByText('Cancel upload',{exact:true})).toBeVisible();
  await expect(page.locator('.xterm-bg-62').filter({hasText:'Cancel upload'})).toBeVisible();
  await clickControl(page,'Cancel upload');
  await expect(screen(page)).toContainText('Background darkness:');
  await clickControl(page,'Back');
  await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(false);
  expect(sockets).toBe(connectedSockets);
});

test('disconnect retires an editor and its prepared URLs without changing the selected background',async({page,baseURL})=>{
  await page.addInitScript(()=>{
    const NativeWebSocket=WebSocket;
    window.WebSocket=class extends NativeWebSocket{constructor(url,...args){super(url,...args);if(new URL(url).pathname==='/ws')window.trimSocket=this;}};
    window.trimURLs=new Set();
    const create=URL.createObjectURL.bind(URL),revoke=URL.revokeObjectURL.bind(URL);
    URL.createObjectURL=blob=>{const url=create(blob);window.trimURLs.add(url);return url;};
    URL.revokeObjectURL=url=>{window.trimURLs.delete(url);revoke(url);};
  });
  // Persist a selected background independently of preceding tests, then hold
  // its restoration so Connected cannot stand in for decoded asset readiness.
  const image = await page.evaluate(() => {
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 16;
    canvas.getContext('2d').fillRect(0, 0, 16, 16);
    return canvas.toDataURL('image/png').split(',')[1];
  });
  const saved = await page.request.post('/visual/background', {headers:{Origin:new URL(baseURL).origin}, multipart:{
    image:{name:'selected.png',mimeType:'image/png',buffer:Buffer.from(image,'base64')},
    darkness:'70',framing:JSON.stringify({fit:'fill',zoom:100,positionX:5000,positionY:5000})
  }});
  expect(saved.ok()).toBe(true);
  let releaseRestore, restoreRequested;
  const restoreGate = new Promise(resolve => {releaseRestore = resolve;});
  const requested = new Promise(resolve => {restoreRequested = resolve;});
  await page.route('**/visual/background', async route => {restoreRequested();await restoreGate;await route.continue();});
  await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
  await requested;
  const before=await (await page.request.get('/visual')).json();
  releaseRestore();
  await expect(page.locator('#background-image')).toBeVisible();
  const activeURLs=await page.evaluate(()=>[...window.trimURLs].sort());
  await openVisual(page);
  const staged=page.waitForRequest(request=>new URL(request.url()).pathname==='/visual/video/source'&&request.method()==='POST');
  await stageSource(page);

  // Keep the initial preview short; no source is retained once the session ends.
  await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');await page.keyboard.press('Home');
  await clickControl(page,'Preview loop');
  const editorID=(await staged).headers()['x-visual-editor'];
  await expect(screen(page)).toContainText('Range: 00:00:00 - 00:00:01');
  await page.emulateMedia({reducedMotion:'reduce'});
  await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.paused)).toBe(true);
  await page.evaluate(()=>window.trimSocket.close());
  await expect(page.locator('#reconnect')).toBeVisible();
  await expect(page.locator('#background-preview-video')).toHaveCount(0);
  await expect.poll(()=>page.evaluate(()=>[...window.trimURLs].sort())).toEqual(activeURLs);
  const retired=await page.request.delete('/visual/video/source',{headers:{Origin:new URL(page.url()).origin,'X-Visual-Editor':editorID}});
  expect(retired.status()).toBe(403);
  expect(await (await page.request.get('/visual')).json()).toEqual(before);
  await page.locator('#reconnect').click();await expect(page.locator('#status')).toHaveText('Connected');
});


test('a decoded loop stays applicable with reduced motion or rejected preview playback',async({page})=>{
  await page.addInitScript(()=>{
    window.previewPlayAttempts=0;
    const play=HTMLMediaElement.prototype.play;
    HTMLMediaElement.prototype.play=function(...args){
      if(this.id==='background-preview-video'){
        window.previewPlayAttempts++;
        return Promise.reject(new DOMException('synthetic blocked preview','NotAllowedError'));
      }
      return play.apply(this,args);
    };
  });
  await page.emulateMedia({reducedMotion:'reduce'});
  await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
  await openVisual(page);await stageSource(page);
  await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');await page.keyboard.press('Home');
  await clickControl(page,'Preview loop');
  await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
  await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.paused)).toBe(true);
  expect(await page.evaluate(()=>window.previewPlayAttempts)).toBe(0);
  await clickControl(page,'Done — use previewed loop');
  await expect(screen(page)).toContainText('Background darkness:');
  await clickControl(page,'Play / pause preview');
  await expect(page.locator('#video-status')).toHaveText('Preview playback is unavailable. The prepared loop is still ready to apply.');
  await expect(page.locator('#background-preview-video')).toBeHidden();
  await expect(page.locator('#background-preview')).toBeVisible();
  expect(await page.evaluate(()=>window.previewPlayAttempts)).toBe(1);
  await clickControl(page,'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  const settings=await (await page.request.get('/visual')).json();
  expect(settings.kind).toBe('video');expect(settings.userPaused).toBe(false);
  await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(true);
  await expect(page.locator('#background-image')).toBeVisible();
});


test('four-hour source validates once before cutting and reuses staging for repeated previews',async({page},testInfo)=>{
 let uploads=0,prepares=0,sockets=0;
 page.on('websocket',()=>sockets++);
 page.on('request',request=>{const url=new URL(request.url());if(request.method()==='POST'&&url.pathname==='/visual/video/source')uploads++;if(request.method()==='POST'&&url.pathname==='/visual/video/preview')prepares++;});
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await openVisual(page);
 const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');await(await chooser).setFiles(longSource);
 await expect(screen(page)).toContainText('Source / Start: 00:00:00.000 / 04:00:00.000');
 // Admission can retry while the preceding test session is closing.
 // Once the editor is active, scrubbing must keep that accepted connection.
 const connectedSockets = sockets;
 expect(uploads).toBe(1);expect(prepares).toBe(0);
 await page.keyboard.press('Enter');await page.keyboard.press('Control+u');await page.keyboard.insertText('03:24:10.000');await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('End: 03:24:40.000');
 await expect.poll(()=>page.locator('#background-preview-video').evaluate(v=>v.currentTime)).toBe(12250);
 await expect(page.locator('#background-preview-video')).toBeVisible();
 await page.screenshot({path:`/tmp/long-source-${testInfo.project.name}-paused.png`});
 const previewHeight=await page.locator('#background-preview-wrap').evaluate(element=>element.getBoundingClientRect().height);
 await page.setViewportSize({width:650,height:600});
 await expect(page.locator('#background-preview-wrap')).toBeVisible();
 await expect.poll(()=>page.locator('#background-preview-wrap').evaluate(element=>element.getBoundingClientRect().height)).toBe(previewHeight);
 await page.keyboard.press('Shift+Tab');await expect(screen(page)).toContainText('Cancel upload');
 await page.keyboard.press('Tab');
 // Controls scroll while the full preview and source position remain stable.
 await page.setViewportSize({width:650,height:750});
 await expect(screen(page)).toContainText('Source / Start: 03:24:10.000');
 await expect.poll(()=>page.locator('#background-preview-wrap').evaluate(element=>{const rect=element.getBoundingClientRect();return !element.hidden&&rect.left>=0&&rect.right<=innerWidth;})).toBe(true);
 await page.screenshot({path:`/tmp/long-source-${testInfo.project.name}-narrow.png`});
 await page.keyboard.press('PageDown');await expect(screen(page)).toContainText('Source / Start: 03:25:10.000');
 await page.keyboard.press('End');await expect(screen(page)).toContainText('Source / Start: 03:59:59.000');
 await expect.poll(()=>page.locator('#background-preview-video').evaluate(v=>v.currentTime)).toBe(14399);
 expect(uploads).toBe(1);expect(prepares).toBe(0);
 await page.keyboard.press('Shift+ArrowLeft');
 await clickControl(page,'Preview loop');await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 expect(uploads).toBe(1);expect(prepares).toBe(1);
 await page.keyboard.press('Tab');await page.keyboard.press('Tab');
 await clickControl(page,'Source / Start: 03:59:58.000 / 04:00:00.000');
 await page.keyboard.press('Shift+ArrowLeft');
 await expect(screen(page)).toContainText('Source / Start: 03:59:57.000');
 const reused=page.waitForResponse(r=>new URL(r.url()).pathname==='/visual/video/preview'&&r.request().method()==='POST');
 await clickControl(page,'Preview loop');await reused;
 await expect(page.locator('#video-preparation')).toBeHidden();
 await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 expect(uploads).toBe(1);expect(prepares).toBe(2);expect(sockets).toBe(connectedSockets);
 expect(await page.locator('#background-preview-video').evaluate(video=>video.duration)).toBe(3);
 await page.keyboard.press('Tab');
 await clickControl(page,'Cancel upload');await expect(screen(page)).toContainText('Background darkness:');
});

test('source rejection is visible before cutting and a valid upload can retry',async({page},testInfo)=>{
 let finishValidation, validationRequested;
 const validationGate=new Promise(resolve=>{finishValidation=resolve;});
 const requested=new Promise(resolve=>{validationRequested=resolve;});
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await openVisual(page);
 await page.route('**/visual/video/source',async route=>{
  if(route.request().method()==='POST') {
   validationRequested();await validationGate;
   await route.fulfill({status:400,headers:{'X-Video-Rejection':'sourceColor'},body:'Private decoder text must not be displayed.'});
  } else await route.continue();
 });
 try {
  const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');await(await chooser).setFiles(source);
  await requested;
  await expect(screen(page)).toContainText('Checking video compatibility… Esc: cancel');
  await expect(screen(page).getByText('Preview loop',{exact:true})).toHaveCSS('color','rgb(153, 153, 153)');
  await clickControl(page,'Preview loop');
  await expect(screen(page)).toContainText('Checking video compatibility… Esc: cancel');
  finishValidation();
  await expect(screen(page)).toContainText('Unsupported HDR/color format. Use supported SDR Rec.709.');
  await expect(screen(page)).not.toContainText('Private decoder text');
  await expect(screen(page)).not.toContainText('Done — use previewed loop');
  await page.setViewportSize({width:650,height:600});
  await expect(screen(page)).toContainText('Unsupported HDR/color format. Use supported SDR Rec.709.');
  await page.screenshot({path:`/tmp/video-rejection-${testInfo.project.name}.png`});
 } finally {finishValidation();await page.unrouteAll({behavior:'wait'});}
 await page.setViewportSize({width:1200,height:850});
 await stageSource(page);
 await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');await page.keyboard.press('Home');
 await clickControl(page,'Preview loop');
 await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await clickControl(page,'Done — use previewed loop');
 await expect(screen(page)).toContainText('Background darkness:');
});

for(const phase of ['source transfer','preview acknowledgement','stopping acknowledgement'])test(`cancellation during ${phase} retires the candidate without changing the background`,async({page})=>{
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 const before=await(await page.request.get('/visual')).json();
 await openVisual(page);
 if(phase!=='source transfer') {
  await stageSource(page);
  await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');await page.keyboard.press('Home');
 }
 let release,entered;
 const gate=new Promise(done=>{release=done;}),arrival=new Promise(done=>{entered=done;});
 const path=phase==='source transfer'?'**/visual/video/source':'**/visual/video/preview';
 await page.route(path,async route=>{
  const method=phase==='source transfer'?'POST':'PUT';
  if(route.request().method()===method){entered();await gate;}
  await route.continue();
 });
 try {
  if(phase==='source transfer') {
   const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');await(await chooser).setFiles(source);
  } else await clickControl(page,'Preview loop');
  await arrival;
  if(phase==='stopping acknowledgement') {await clickControl(page,'Stop rendering');await expect(screen(page)).toContainText('Stopping rendering');}
  if(phase!=='source transfer') await clickControl(page,'Cancel upload');
  else await page.keyboard.press('Escape');
  if(phase!=='source transfer') {
   await expect(screen(page)).toContainText('Background darkness:');
  } else await expect(screen(page)).not.toContainText('Checking video compatibility');
  expect(await(await page.request.get('/visual')).json()).toEqual(before);
  await expect(page.locator('#background-preview-video')).toHaveCount(0);
 }finally{release();await page.unrouteAll({behavior:'wait'});}
});

test('nonzero timestamp sources preview and prepare the same selected scene',async({page})=>{
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await openVisual(page);
 const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');await(await chooser).setFiles(offsetSource);
 await expect(screen(page)).toContainText('Source / Start: 00:00:00.000 / 00:00:12.000');
 const sourceOrigin=await page.locator('#background-preview-video').evaluate(video=>video.currentTime);
 await page.keyboard.press('Enter');await page.keyboard.press('Control+u');await page.keyboard.insertText('00:00:10.250');await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('Source / Start: 00:00:10.250');
 const preview=page.locator('#background-preview-video');
 const brightness=()=>preview.evaluate(video=>{
  if(video.seeking||video.readyState<2)return -1;
  const canvas=document.createElement('canvas');canvas.width=1;canvas.height=1;
  const context=canvas.getContext('2d');context.drawImage(video,0,0,1,1);
  return context.getImageData(0,0,1,1).data[0];
 });
 // This synthetic source brightens each second. A five-second timestamp error
 // changes brightness by about 70 levels, far beyond codec rounding.
 if(sourceOrigin>0) {
  await expect(page.locator('#source-preview-status')).toBeVisible();
 } else {
  await expect.poll(brightness).toBeGreaterThan(154);
  await expect.poll(brightness).toBeLessThan(172);
 }
 await clickControl(page,'End: 00:00:12.000 (max 00:00:12.000)');await page.keyboard.press('Home');
 await clickControl(page,'Preview loop');
 await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await expect(screen(page)).toContainText('Range: 00:00:10.250 - 00:00:11.250');
 await expect(page.locator('#source-preview-status')).toBeHidden();
 await expect.poll(brightness).toBeGreaterThan(154);
 await expect.poll(brightness).toBeLessThan(172);
 await clickControl(page,'Done — use previewed loop');
 await expect(screen(page)).toContainText('Background darkness:');
 await clickControl(page,'Apply');
 await expect(screen(page)).not.toContainText('Background darkness:');
 expect((await(await page.request.get('/visual')).json()).kind).toBe('video');
});


test('fragmented MP4 validates up front and prepares a sixteen-second background',async({page})=>{
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await openVisual(page);
 const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');await(await chooser).setFiles(fragmentedSource);
 await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
 await clickControl(page,'Source / Start: 00:00:00.000 / 00:01:00.000');
 await page.keyboard.press('Enter');await page.keyboard.press('Control+u');await page.keyboard.insertText('00:00:10.000');await page.keyboard.press('Enter');
 await clickControl(page,'End: 00:00:40.000 (max 00:00:40.000)');
 await page.keyboard.press('Home');
 for(let second=0;second<15;second++)await page.keyboard.press('Shift+ArrowRight');
 await expect(screen(page)).toContainText('Selected: 16.0 seconds');
 await clickControl(page,'Preview loop');
 await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await expect(screen(page)).toContainText('Range: 00:00:10 - 00:00:26');
 await clickControl(page,'Done — use previewed loop');
 await clickControl(page,'Apply');
 await expect(screen(page)).not.toContainText('Background darkness:');
 expect((await(await page.request.get('/visual')).json()).kind).toBe('video');
});


for (const forceFallback of [false,true]) test(`Matroska scene browsing and exact Apply with forced fallback ${forceFallback}`,async({page},testInfo)=>{
 let uploads=0,frames=0,activeFrames=0,maximumFrames=0,preparations=0,sockets=0;
 page.on('websocket',()=>sockets++);
 page.on('request',request=>{
  if(request.method()!=='POST')return;
  const path=new URL(request.url()).pathname;
  if(path==='/visual/video/source')uploads++;
  if(path==='/visual/video/preview')preparations++;
  if(path==='/visual/video/frame'){frames++;activeFrames++;maximumFrames=Math.max(maximumFrames,activeFrames);}
 });
 const finished=request=>{if(new URL(request.url()).pathname==='/visual/video/frame')activeFrames--;};
 page.on('requestfinished',finished);page.on('requestfailed',finished);
 if(forceFallback)await page.addInitScript(()=>{
  const sources=new Set(),create=URL.createObjectURL;
  URL.createObjectURL=function(blob){const url=create.call(this,blob);if(blob.type==='video/x-matroska')sources.add(url);return url;};
  const load=HTMLMediaElement.prototype.load;
  HTMLMediaElement.prototype.load=function(){if(sources.has(this.src)){queueMicrotask(()=>this.dispatchEvent(new Event('error')));return;}return load.call(this);};
 });
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await openVisual(page);
 const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');await(await chooser).setFiles(matroskaSource);
 await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
 if(forceFallback)await expect(page.locator('#source-preview-status')).toContainText('Native source preview');
 await page.keyboard.press('Enter');await page.keyboard.press('Control+u');await page.keyboard.insertText('00:00:04.200');await page.keyboard.press('Enter');
 const brightness=()=>page.evaluate(()=>{
  const video=document.querySelector('#background-preview-video');
  const image=document.querySelector('#background-preview');
  const element=video&&!video.hidden?video:image;
  if(element instanceof HTMLVideoElement&&(element.seeking||element.readyState<2))return -1;
  if(element instanceof HTMLImageElement&&(!element.complete||!element.naturalWidth))return -1;
  const canvas=document.createElement('canvas');canvas.width=canvas.height=1;
  const context=canvas.getContext('2d');context.drawImage(element,0,0,1,1);return context.getImageData(0,0,1,1).data[0];
 });
 await expect.poll(brightness).toBeGreaterThan(60);await expect.poll(brightness).toBeLessThan(77);
 for(let i=0;i<20;i++)await page.keyboard.press('ArrowRight');
 await expect(screen(page)).toContainText('Source / Start: 00:00:06.200');
 await expect.poll(brightness).toBeGreaterThan(84);await expect.poll(brightness).toBeLessThan(101);
 if(forceFallback){expect(frames).toBeGreaterThan(0);expect(frames).toBeLessThan(8);expect(maximumFrames).toBe(1);}
 await expect(page.locator('#background-preview-wrap')).toBeVisible();
 await page.screenshot({path:`/tmp/mkv-${testInfo.project.name}-${forceFallback}.png`});
 if(forceFallback){
  await clickControl(page,'End: 00:00:12.023 (max 00:00:12.023)');
  await page.keyboard.press('End');
  await expect.poll(brightness).toBeGreaterThan(146);
  await page.setViewportSize({width:650,height:850});
  await expect(page.locator('#background-preview-wrap')).toBeVisible();
  await page.screenshot({path:`/tmp/mkv-${testInfo.project.name}-narrow.png`});
  await page.setViewportSize({width:1200,height:850});
 }
 await clickControl(page,'Preview loop');await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await expect.poll(brightness).toBeGreaterThan(84);await expect.poll(brightness).toBeLessThan(101);
 await clickControl(page,'Done — use previewed loop');await clickControl(page,'Apply');
 await expect(screen(page)).not.toContainText('Background darkness:');
 expect((await(await page.request.get('/visual')).json()).kind).toBe('video');
 expect(uploads).toBe(1);expect(preparations).toBe(1);expect(sockets).toBe(1);
});


test('browser decode failure falls back for a verified single-track Matroska source',async({page})=>{
 let frames=0;
 page.on('request',request=>{if(new URL(request.url()).pathname==='/visual/video/frame')frames++;});
 await page.addInitScript(()=>{
  const urls=new Set(),create=URL.createObjectURL,load=HTMLMediaElement.prototype.load;
  URL.createObjectURL=function(blob){const url=create.call(this,blob);if(blob.type==='video/x-matroska')urls.add(url);return url;};
  HTMLMediaElement.prototype.load=function(){if(urls.has(this.src)){queueMicrotask(()=>this.dispatchEvent(new Event('error')));return;}return load.call(this);};
 });
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');await openVisual(page);
 const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');await(await chooser).setFiles(singleVideoMatroska);
 await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
 await expect(page.locator('#source-preview-status')).toContainText('Native source preview');
 await expect(page.locator('#background-preview-wrap')).toBeVisible();
 await expect(page.locator('#background-preview')).toBeVisible();
 await expect.poll(()=>frames).toBeGreaterThan(0);
 await expect.poll(()=>page.locator('#background-preview').evaluate(image=>[image.naturalWidth,image.naturalHeight])).toEqual([96,64]);
 await clickControl(page,'Preview loop');await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await clickControl(page,'Done — use previewed loop');await clickControl(page,'Apply');
 await expect(screen(page)).not.toContainText('Background darkness:');
});

for(const stop of ['Escape','click'])test(`Stop rendering via ${stop} retains upload and accepted loop for retry`,async({page},testInfo)=>{
 let uploads=0,preparations=0,sockets=0;
 page.on('websocket',()=>sockets++);
 page.on('request',request=>{if(request.method()==='POST'){const path=new URL(request.url()).pathname;if(path==='/visual/video/source')uploads++;if(path==='/visual/video/preview')preparations++;}});
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');await openVisual(page);await stageSource(page);
 await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');await page.keyboard.press('Home');
 await clickControl(page,'Preview loop');await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await clickControl(page,'End: 00:00:01.000 (max 00:00:30.000)');await page.keyboard.press('ArrowRight');
 let release,entered;
 const gate=new Promise(resolve=>{release=resolve;}),arrival=new Promise(resolve=>{entered=resolve;});
 await page.route('**/visual/video/preview',async route=>{if(route.request().method()==='POST'){entered();await gate;}await route.continue();});
 try {
  await clickControl(page,'Preview loop');await arrival;
  await expect(screen(page)).toContainText('Stop rendering');
  await expect(screen(page).getByText('Done — use previewed loop',{exact:true})).toHaveCSS('color','rgb(153, 153, 153)');
  await clickControl(page,'Done — use previewed loop');
  await expect(screen(page)).toContainText('Stop rendering');
  const rows=await screen(page).locator(':scope > div').allTextContents(),cancel=rows.findIndex(row=>row.trim()==='Cancel upload');
  expect(cancel).toBeGreaterThan(0);expect(rows[cancel-1].trim()).toBe('');
  if(process.env.THOUGHTS_BROWSER_ARTIFACTS)await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-render-stop.png`});
  if(stop==='Escape')await page.keyboard.press('Escape');else await clickControl(page,'Stop rendering');
  await expect(screen(page)).toContainText('Rendering stopped.');
  await expect(screen(page)).toContainText('Done — use previewed loop');await expect(screen(page)).toContainText('Selected: 1.1 seconds');
  expect(uploads).toBe(1);expect(sockets).toBe(1);
 } finally {release();await page.unrouteAll({behavior:'wait'});}
 await clickControl(page,'Preview loop');await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await clickControl(page,'Done — use previewed loop');await clickControl(page,'Apply');await expect(screen(page)).not.toContainText('Background darkness:');
 expect(uploads).toBe(1);expect(preparations).toBe(3);expect(sockets).toBe(1);
});

test('complete source and prepared movie frames fit the preview after narrow resize', async ({page}, testInfo) => {
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await openVisual(page);await stageSource(page);
 const completeFrame=()=>page.evaluate(()=>{
  const frame=document.getElementById('background-preview-frame').getBoundingClientRect();
  const media=[...document.querySelectorAll('#background-preview-frame video, #background-preview')].find(element=>!element.hidden);
  if(!media)return false;
  const bounds=media.getBoundingClientRect();
  const ratio=(media.videoWidth||media.naturalWidth)/(media.videoHeight||media.naturalHeight);
  return bounds.width>0 && bounds.height>0 && Math.abs(frame.width/frame.height-ratio)<0.01 &&
   bounds.left>=frame.left-1 && bounds.right<=frame.right+1 && bounds.top>=frame.top-1 && bounds.bottom<=frame.bottom+1;
 });
 await expect.poll(completeFrame).toBe(true);
 await page.setViewportSize({width:650,height:850});
 await expect.poll(completeFrame).toBe(true);
 if(process.env.THOUGHTS_BROWSER_ARTIFACTS)await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-full-source-narrow.png`});
 await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');await page.keyboard.press('Home');
 await clickControl(page,'Preview loop');await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await expect.poll(completeFrame).toBe(true);
 if(process.env.THOUGHTS_BROWSER_ARTIFACTS)await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-full-loop-narrow.png`});
 await page.setViewportSize({width:1200,height:850});
 await expect.poll(completeFrame).toBe(true);
});

for(const cancel of [false,true])test(`upload validation shares rendering activity placement and remains cancellable ${cancel}`,async({page},testInfo)=>{
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');await openVisual(page);
 if(cancel)await page.setViewportSize({width:650,height:600});
 const saved=await(await page.request.get('/visual')).json();
 let releaseUpload,releaseRender;
 const uploadGate=new Promise(resolve=>{releaseUpload=resolve;}),renderGate=new Promise(resolve=>{releaseRender=resolve;});
 await page.route('**/visual/video/source',async route=>{if(route.request().method()==='POST')await uploadGate;await route.continue();});
 await page.route('**/visual/video/preview',async route=>{if(route.request().method()==='POST')await renderGate;await route.continue();});
 try {
  const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');await(await chooser).setFiles(source);
  await expect(screen(page)).toContainText('Checking video compatibility');
  const wave=page.locator('#video-preparation-wave');
  if(cancel && testInfo.project.name==='firefox'){
   // At 600px Firefox has only one wave row after the full preview and pinned footer.
   // Keep native status/Cancel visible rather than overlaying those controls.
   await expect(wave).toBeHidden();
  }else{await expect(wave).toBeVisible();await expect(wave).toBeInViewport();}
  await expect(page.locator('#video-preparation-label')).toHaveText('Uploading and validating video…');
  await expect(screen(page)).toContainText('Cancel upload');
  if(cancel)await expect(screen(page)).not.toContainText('Source / Start:');
  else await expect(screen(page)).toContainText('Source / Start: — / —');
  await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
  const duringUpload=await wave.boundingBox();
  if(process.env.THOUGHTS_BROWSER_ARTIFACTS)await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-upload-wave-${cancel?"narrow":"desktop"}.png`});
  if(cancel){
   await clickControl(page,'Cancel upload');await expect(wave).toBeHidden();
   await expect(screen(page)).toContainText('Background darkness:');
   expect(await(await page.request.get('/visual')).json()).toEqual(saved);
  }else{
   releaseUpload();await expect(screen(page)).toContainText('Set cut points, then Preview loop.');await expect(wave).toBeHidden();
   await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');await page.keyboard.press('Home');await clickControl(page,'Preview loop');
   await expect(wave).toBeVisible();
   await expect.poll(async()=>{const bounds=await wave.boundingBox();return bounds&&Math.abs(bounds.y-duringUpload.y)<1&&Math.abs(bounds.x-duringUpload.x)<1&&Math.abs(bounds.width-duringUpload.width)<1&&Math.abs(bounds.height-duringUpload.height)<1;}).toBe(true);
   await page.emulateMedia({reducedMotion:'reduce'});await expect(wave).toHaveAttribute('data-animating','false');
   await page.keyboard.press('Escape');await expect(screen(page)).toContainText('Rendering stopped.');
  }
 } finally {releaseUpload();releaseRender();await page.unrouteAll({behavior:'wait'});}
});

test('short movie editor scrolls controls without shrinking preview or losing accepted work',async({page},testInfo)=>{
 let uploads=0,prepares=0,sockets=0;
 page.on('websocket',()=>sockets++);
 page.on('request',request=>{if(request.method()==='POST'){const path=new URL(request.url()).pathname;if(path==='/visual/video/source')uploads++;if(path==='/visual/video/preview')prepares++;}});
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');await openVisual(page);await stageSource(page);
 const preview=page.locator('#background-preview-wrap');
 const normal=await preview.boundingBox();
 await page.setViewportSize({width:650,height:600});
 await expect(preview).toBeVisible();await expect.poll(async()=>(await preview.boundingBox())?.height).toBe(normal.height);
 await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');await page.keyboard.press('Home');
 await page.keyboard.press('Shift+Tab');await page.keyboard.press('Shift+Tab');
 await expect(screen(page)).toContainText('Cancel upload');
 await expect(screen(page)).not.toContainText('Source / Start:');
 await expect.poll(async()=>(await preview.boundingBox())?.y).toBe(normal.y);
 if(process.env.THOUGHTS_BROWSER_ARTIFACTS)await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-scroll-controls.png`});
 await clickControl(page,'Preview loop');await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 await page.locator('#background-preview-video').evaluate(video=>{window.acceptedBeforeResize=video;});
 await page.setViewportSize({width:400,height:300});
 await expect(screen(page)).toContainText('Resize window');await expect(preview).toBeHidden();
 if(process.env.THOUGHTS_BROWSER_ARTIFACTS)await page.screenshot({path:`${process.env.THOUGHTS_BROWSER_ARTIFACTS}/${testInfo.project.name}-resize-notice.png`});
 await page.keyboard.press('ArrowRight');
 await page.setViewportSize({width:650,height:600});
 await expect(preview).toBeVisible();await expect.poll(async()=>(await preview.boundingBox())?.height).toBe(normal.height);
 await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video===window.acceptedBeforeResize)).toBe(true);
 await expect(screen(page)).toContainText('Range: 00:00:00 - 00:00:01');
 expect(uploads).toBe(1);expect(prepares).toBe(1);expect(sockets).toBe(1);
 await page.setViewportSize({width:1200,height:850});
 await clickControl(page,'Done — use previewed loop');await clickControl(page,'Apply');
 await expect(screen(page)).not.toContainText('Background darkness:');
 expect(prepares).toBe(1);
});
