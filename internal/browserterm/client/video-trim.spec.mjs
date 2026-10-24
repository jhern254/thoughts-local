import {test, expect} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {mkdtempSync, readFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {openVisual, clickControl, screen} from './visual-helpers.mjs';

let source, longSource, offsetSource;
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
    const longPath=join(directory,'four-hours.mp4');
    execFileSync('ffmpeg',['-v','error','-f','lavfi','-i','testsrc2=size=32x24:rate=1:duration=14400','-c:v','libx264','-threads','1','-g','60','-pix_fmt','yuv420p','-movflags','+faststart',longPath]);
    longSource={name:'four-hours.mp4',mimeType:'video/mp4',buffer:readFileSync(longPath)};
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

test('preparing a loop reserves a braille wave between speed and Preview loop and keeps keyboard cancellation available',async({page},testInfo)=>{
  let sockets=0, releasePreparation;
  page.on('websocket',()=>sockets++);
  await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
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
    await expect.poll(async () => (await previewControl.boundingBox())?.y).toBe(previewBefore.y);
    expect(await page.locator('.xterm-screen').boundingBox()).toEqual(terminalBefore);
    await expect(done).toHaveCSS('color', 'rgb(153, 153, 153)');
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
        const speed=rows.find(row=>row.textContent.includes('Playback speed:')).getBoundingClientRect();
        const action=rows.find(row=>row.textContent.trim()==='Preview loop').getBoundingClientRect();
        const aligned=!preview.width || (Math.abs(wave.left-preview.left)<1 && Math.abs(wave.width-preview.width)<1);
        return Math.abs(indicator.height-expectedHeight)<1 && wave.top>=speed.bottom && wave.bottom<=action.top+1 && aligned;
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
    const previewPending = await previewControl.boundingBox();
    releasePreparation();
    await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
    await expect.poll(async () => (await previewControl.boundingBox())?.y).toBe(previewPending.y);
    await expect(done).not.toHaveCSS('color', 'rgb(153, 153, 153)');
    await page.screenshot({path:`/tmp/video-trim-ready-${testInfo.project.name}.png`});
    await expect(bar).toBeHidden();
    expect(await bar.evaluate(element=>element.getAnimations({subtree:true}).length)).toBe(0);
    const retiredFrame=await lightPosition();
    await page.waitForTimeout(300);
    expect(await lightPosition()).toBe(retiredFrame);
    await page.unroute('**/visual/video/preview');

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
      await expect(screen(page)).toContainText('Could not load or apply.');
      await expect(bar).toBeHidden();
      await expect(screen(page)).toContainText('Range: 0.00–1.00s');
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
      expect(sockets).toBe(1);
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
    await page.keyboard.press('Escape');
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
  await expect(screen(page)).toContainText('Range: 57.00–60.00s');
  await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.paused)).toBe(false);
  expect(await page.locator('#background-preview-video').evaluate(video=>video.duration)).toBe(3);
  // Capture the actual fetched bytes before they become a media blob.
  // CSP deliberately permits blob playback, not arbitrary blob fetch requests.
  const previewBytes=Buffer.from(await page.evaluate(()=>window.trimPreviewBytes));
  await page.screenshot({path:`/tmp/trim-${testInfo.project.name}-desktop.png`});
  await clickControl(page,'Source / Start: 00:00:57.000 / 00:01:00.000');await page.keyboard.press('Home');
  await expect(screen(page)).toContainText('End: 00:00:30.000 (max 00:00:30.000)');
  await expect(screen(page)).toContainText('Range: 57.00–60.00s');
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
  await expect(screen(page)).toContainText('Could not load or apply.');
  await expect(screen(page)).toContainText('Range: 57.00–60.00s');
  expect(await page.locator('#background-preview-video').evaluate(video=>video.duration)).toBe(3);
  await page.unroute('**/visual/video/preview');
  await page.setViewportSize({width:650,height:600});
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
  await expect(screen(page)).toContainText('Range: 0.00–1.00s');
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


test('four-hour source stays local while Start seeks and uploads once for repeated previews',async({page},testInfo)=>{
 let uploads=0,prepares=0,sockets=0;
 page.on('websocket',()=>sockets++);
 page.on('request',request=>{const url=new URL(request.url());if(request.method()==='POST'&&url.pathname==='/visual/video/source')uploads++;if(request.method()==='POST'&&url.pathname==='/visual/video/preview')prepares++;});
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await openVisual(page);
 const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');await(await chooser).setFiles(longSource);
 await expect(screen(page)).toContainText('Source / Start: 00:00:00.000 / 04:00:00.000');
 expect(uploads).toBe(0);expect(prepares).toBe(0);
 await page.keyboard.press('Enter');await page.keyboard.press('Control+u');await page.keyboard.insertText('03:24:10.000');await page.keyboard.press('Enter');
 await expect(screen(page)).toContainText('End: 03:24:40.000');
 await expect.poll(()=>page.locator('#background-preview-video').evaluate(v=>v.currentTime)).toBe(12250);
 await expect(page.locator('#background-preview-video')).toBeVisible();
 await page.screenshot({path:`/tmp/long-source-${testInfo.project.name}-paused.png`});
 await page.setViewportSize({width:650,height:600});
 await expect(screen(page)).toContainText('Source / Start: 03:24:10.000');
 await expect.poll(()=>page.locator('#background-preview-wrap').evaluate(element=>{const rect=element.getBoundingClientRect();return !element.hidden&&rect.left>=0&&rect.right<=innerWidth;})).toBe(true);
 await page.screenshot({path:`/tmp/long-source-${testInfo.project.name}-narrow.png`});
 await page.keyboard.press('PageDown');await expect(screen(page)).toContainText('Source / Start: 03:25:10.000');
 await page.keyboard.press('End');await expect(screen(page)).toContainText('Source / Start: 03:59:59.000');
 await expect.poll(()=>page.locator('#background-preview-video').evaluate(v=>v.currentTime)).toBe(14399);
 expect(uploads).toBe(0);expect(prepares).toBe(0);
 await page.keyboard.press('Shift+ArrowLeft');
 await clickControl(page,'Preview loop');await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 expect(uploads).toBe(1);expect(prepares).toBe(1);
 await clickControl(page,'Source / Start: 03:59:58.000 / 04:00:00.000');
 await page.keyboard.press('Shift+ArrowLeft');
 await expect(screen(page)).toContainText('Source / Start: 03:59:57.000');
 const reused=page.waitForResponse(r=>new URL(r.url()).pathname==='/visual/video/preview'&&r.request().method()==='POST');
 await clickControl(page,'Preview loop');await reused;
 await expect(page.locator('#video-preparation')).toBeHidden();
 await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
 expect(uploads).toBe(1);expect(prepares).toBe(2);expect(sockets).toBe(1);
 expect(await page.locator('#background-preview-video').evaluate(video=>video.duration)).toBe(3);
 await clickControl(page,'Cancel upload');await expect(screen(page)).toContainText('Background darkness:');
});

for(const phase of ['source transfer','preview acknowledgement'])test(`cancellation during ${phase} retires the candidate without changing the background`,async({page})=>{
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 const before=await(await page.request.get('/visual')).json();
 await openVisual(page);await stageSource(page);
 await clickControl(page,'End: 00:00:30.000 (max 00:00:30.000)');await page.keyboard.press('Home');
 let release,entered;
 const gate=new Promise(done=>{release=done;}),arrival=new Promise(done=>{entered=done;});
 const path=phase==='source transfer'?'**/visual/video/source':'**/visual/video/preview';
 await page.route(path,async route=>{
  const method=phase==='source transfer'?'POST':'PUT';
  if(route.request().method()===method){entered();await gate;}
  await route.continue();
 });
 try {
  await clickControl(page,'Preview loop');await arrival;
  await page.keyboard.press('Escape');await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('Background darkness:');
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
 await expect(screen(page)).toContainText('Range: 10.25–11.25s');
 await expect(page.locator('#source-preview-status')).toBeHidden();
 await expect.poll(brightness).toBeGreaterThan(154);
 await expect.poll(brightness).toBeLessThan(172);
 await clickControl(page,'Done — use previewed loop');
 await expect(screen(page)).toContainText('Background darkness:');
 await clickControl(page,'Apply');
 await expect(screen(page)).not.toContainText('Background darkness:');
 expect((await(await page.request.get('/visual')).json()).kind).toBe('video');
});
