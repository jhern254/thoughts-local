import {test, expect} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {mkdtempSync, readFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {openVisual, clickControl, screen} from './visual-helpers.mjs';

let source;
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
  } finally {rmSync(directory,{recursive:true,force:true});}
});

async function stageSource(page) {
  const chooser = page.waitForEvent('filechooser');
  await clickControl(page,'Choose image or video');
  await (await chooser).setFiles(source);
  await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
  await expect(page.locator('#background-preview-wrap')).toBeVisible();
}

test('preparing a loop shows a braille wave above Options and keeps keyboard cancellation available',async({page},testInfo)=>{
  let sockets=0, releasePreparation;
  page.on('websocket',()=>sockets++);
  await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
  await openVisual(page);await stageSource(page);
  await clickControl(page,'End: 30.00s (max 30.00s)');
  await page.keyboard.press('Home');await page.keyboard.press('Shift+ArrowRight');
  await expect(screen(page)).toContainText('End: 1.00s');
  const gate=new Promise(resolve=>{releasePreparation=resolve;});
  await page.route('**/visual/video/preview',async route=>{
    if(route.request().method()==='POST')await gate;
    await route.continue();
  });
  try {
    await clickControl(page,'Preview loop');
    const bar=page.locator('#video-preparation-wave');
    await expect(page.getByRole('progressbar',{name:'Preparing video…'})).toBeVisible();
    await expect(bar).not.toHaveAttribute('aria-valuenow');
    const lightPosition=()=>bar.locator('.preparation-sweep').first().evaluate(element=>getComputedStyle(element).transform);
    const position=await lightPosition();
    await expect.poll(lightPosition).not.toBe(position);
    await expect(bar.locator('pre')).toHaveCount(2);
    async function expectWavePlacement() {
      await expect.poll(()=>page.evaluate(()=>{
        const wave=document.getElementById('video-preparation-wave').getBoundingClientRect();
        const preview=document.getElementById('background-preview-wrap').getBoundingClientRect();
        const terminal=document.querySelector('.xterm-screen').getBoundingClientRect();
        return wave.bottom<=terminal.top && Math.abs(wave.left-preview.left)<1 && Math.abs(wave.width-preview.width)<1;
      })).toBe(true);
    }
    await expectWavePlacement();
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
    releasePreparation();
    await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
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
  await clickControl(page,'End: 30.00s (max 30.00s)');
  await page.keyboard.press('End');
  await page.keyboard.press('Shift+ArrowRight');
  await expect(screen(page)).toContainText('End: 30.00s (max 30.00s)');
  await page.keyboard.press('Shift+Tab');await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('End');
  await expect(screen(page)).toContainText('Source: 60.00 / 60.00s');
  await clickControl(page,'End: 30.00s (max 30.00s)');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('End: 30.00s (max 30.00s)');
  await page.screenshot({path:`/tmp/trim-${testInfo.project.name}-end-limit.png`});
  await page.keyboard.press('Shift+Tab');await page.keyboard.press('Shift+Tab');
  for(let i=0;i<3;i++)await page.keyboard.press('Shift+ArrowLeft');
  await expect(screen(page)).toContainText('Source: 57.00 / 60.00s');
  await clickControl(page,'Start: 0.00s');await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('End: 30.00s (max 60.00s)');
  await page.keyboard.press('Tab');await page.keyboard.press('End');
  await clickControl(page,'Preview loop');
  await expect(screen(page)).toContainText('Range: 57.00–60.00s');
  await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.paused)).toBe(false);
  expect(await page.locator('#background-preview-video').evaluate(video=>video.duration)).toBe(3);
  // Capture the actual fetched bytes before they become a media blob.
  // CSP deliberately permits blob playback, not arbitrary blob fetch requests.
  const previewBytes=Buffer.from(await page.evaluate(()=>window.trimPreviewBytes));
  await page.screenshot({path:`/tmp/trim-${testInfo.project.name}-desktop.png`});
  await clickControl(page,'Start: 57.00s');await page.keyboard.press('Home');
  await expect(screen(page)).toContainText('End: 30.00s (max 30.00s)');
  await expect(screen(page)).toContainText('Range: 57.00–60.00s');
  expect(prepares).toBe(1);
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Start: 57.00s');
  await page.keyboard.press('Tab');await page.keyboard.press('End');
  await expect(screen(page)).toContainText('End: 60.00s (max 60.00s)');
  // A failed explicit replacement must leave both native acknowledgement and
  // browser player on the previously reviewed loop.
  await clickControl(page,'Start: 57.00s');await page.keyboard.press('ArrowRight');
  await expect(screen(page)).toContainText('Start: 57.10s');
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

test('disconnect retires an editor and its prepared URLs without changing the selected background',async({page})=>{
  await page.addInitScript(()=>{
    const NativeWebSocket=WebSocket;
    window.WebSocket=class extends NativeWebSocket{constructor(url,...args){super(url,...args);if(new URL(url).pathname==='/ws')window.trimSocket=this;}};
    window.trimURLs=new Set();
    const create=URL.createObjectURL.bind(URL),revoke=URL.revokeObjectURL.bind(URL);
    URL.createObjectURL=blob=>{const url=create(blob);window.trimURLs.add(url);return url;};
    URL.revokeObjectURL=url=>{window.trimURLs.delete(url);revoke(url);};
  });
  await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
  const before=await (await page.request.get('/visual')).json();
  const activeURLs=await page.evaluate(()=>window.trimURLs.size);
  await openVisual(page);
  const staged=page.waitForRequest(request=>new URL(request.url()).pathname==='/visual/video/source'&&request.method()==='POST');
  await stageSource(page);
  const editorID=(await staged).headers()['x-visual-editor'];
  // Keep the initial preview short; no source is retained once the session ends.
  await clickControl(page,'End: 30.00s (max 30.00s)');await page.keyboard.press('Home');await page.keyboard.press('Shift+ArrowRight');
  await clickControl(page,'Preview loop');await expect(screen(page)).toContainText('Range: 0.00–1.00s');
  await page.emulateMedia({reducedMotion:'reduce'});
  await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.paused)).toBe(true);
  await page.evaluate(()=>window.trimSocket.close());
  await expect(page.locator('#reconnect')).toBeVisible();
  await expect(page.locator('#background-preview-video')).toHaveCount(0);
  await expect.poll(()=>page.evaluate(()=>window.trimURLs.size)).toBe(activeURLs);
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
  await clickControl(page,'End: 30.00s (max 30.00s)');await page.keyboard.press('Home');await page.keyboard.press('Shift+ArrowRight');
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
