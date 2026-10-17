import {readFileSync} from 'node:fs';
import {test, expect} from '@playwright/test';
import {openVisual, clickControl, chooseImage, screen} from './visual-helpers.mjs';

const videoFixture = {name:'synthetic.mp4', mimeType:'video/mp4', buffer:readFileSync(new URL('./fixtures/visual-video.mp4',import.meta.url))};

test.beforeEach(async ({request, baseURL}) => {
 const response=await request.put('/visual/settings',{headers:{Origin:new URL(baseURL).origin},data:{darkness:70,framing:{fit:'fill',zoom:100,positionX:5000,positionY:5000},speedPercent:100,userPaused:false}});
 expect(response.ok(),`reset visual settings returned HTTP ${response.status()}`).toBe(true);
});

async function selectVideo(page) {
 await openVisual(page); await chooseImage(page,videoFixture);
 await expect(screen(page)).toContainText('Playback speed: 100%');
 await clickControl(page,'Apply');
 await expect(screen(page)).not.toContainText('Background darkness:');
 await expect(page.locator('#background-image')).toBeVisible();
 await expect(page.locator('#background-video')).toBeVisible();
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(false);
}

test('video preserves draft, connection, framing and player across settings-only saves',async({page},testInfo)=>{
 let sockets=0;page.on('websocket',()=>sockets++);
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 const socketsAtStart=sockets;
 await page.keyboard.press('t');await expect(screen(page)).toContainText('Create thought');
 await page.keyboard.insertText('Video keeps this synthetic draft 界');
 await selectVideo(page);
 await expect(screen(page)).toContainText('Video keeps this synthetic draft 界');
 await expect(page.locator('.xterm-helper-textarea')).toBeFocused();
 await page.locator('#background-video').evaluate(video=>{video.dataset.identity='selected';});
 await openVisual(page);await clickControl(page,'Playback speed: 100%');await page.keyboard.press('ArrowLeft');
 await expect(screen(page)).toContainText('Playback speed: 95%');
 await clickControl(page,'Apply');await expect(screen(page)).not.toContainText('Background darkness:');
 expect(await page.locator('#background-video').getAttribute('data-identity')).toBe('selected');
 expect(await page.locator('#background-video').evaluate(video=>video.playbackRate)).toBe(.95);
 expect(sockets).toBe(socketsAtStart);
 await openVisual(page);await clickControl(page,'Play / pause preview');
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(true);
 await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.paused)).toBe(false);
 await clickControl(page,'Back');
 await expect(page.locator('#background-preview-video')).toHaveCount(0);
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(false);
 await page.screenshot({path:`/tmp/video-${testInfo.project.name}-desktop.png`});
 await page.setViewportSize({width:650,height:600});await openVisual(page);
 await expect(screen(page)).toContainText('Playback speed: 95%');
 await page.screenshot({path:`/tmp/video-${testInfo.project.name}-narrow-options.png`});
 // Preview pause is immediate; only closing Options confirms the save completed.
 // Hold the write so an already-paused player cannot stand in for persistence.
 let releasePauseSave;
 const pauseSaveGate = new Promise(resolve => {releasePauseSave = resolve;});
 let pauseSaveStarted = false;
 await page.route('**/visual/settings', async route => {
  if (route.request().method() === 'PUT' && route.request().postDataJSON().userPaused) {
   pauseSaveStarted = true;
   await pauseSaveGate;
  }
  await route.continue();
 });
 await clickControl(page,'Pause video');
 await expect(screen(page)).toContainText('Play video');
 await clickControl(page,'Apply');
 try {
  await expect.poll(()=>pauseSaveStarted).toBe(true);
  await expect(screen(page)).toContainText('Background darkness:');
  expect((await (await page.request.get('/visual')).json()).userPaused).toBe(false);
 } finally {releasePauseSave();}
 await expect(screen(page)).not.toContainText('Background darkness:');
 expect((await (await page.request.get('/visual')).json()).userPaused).toBe(true);
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(true);
 await expect(page.locator('#background-image')).toBeVisible();
 await page.reload();await expect(page.locator('#status')).toHaveText('Connected');
 await expect.poll(()=>page.locator('#background-image').evaluate(image=>image.naturalWidth)).toBe(96);
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(true);
 const settings=await (await page.request.get('/visual')).json();expect(settings.speedPercent).toBe(95);expect(settings.userPaused).toBe(true);
 await openVisual(page);await clickControl(page,'Remove background');
 await expect(screen(page)).not.toContainText('Background darkness:');
 await expect(page.locator('#background-video')).toHaveCount(0);await expect(page.locator('#background-image')).toBeHidden();
});

test('reduced motion and blocked playback keep a stable poster without changing user preference',async({page},testInfo)=>{
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');await selectVideo(page);
 await openVisual(page);await clickControl(page,'Play / pause preview');
 await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.paused)).toBe(false);
 await page.emulateMedia({reducedMotion:'reduce'});
 await expect.poll(()=>page.locator('#background-preview-video').evaluate(video=>video.paused)).toBe(true);
 await clickControl(page,'Back');
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(true);
 await expect(page.locator('#background-video')).toBeHidden();await expect(page.locator('#background-image')).toBeVisible();
 expect((await (await page.request.get('/visual')).json()).userPaused).toBe(false);
 await page.screenshot({path:`/tmp/video-${testInfo.project.name}-reduced-motion.png`});
 await openVisual(page);await clickControl(page,'Play video');
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(false);
 await clickControl(page,'Back');
 await page.reload();await expect(page.locator('#status')).toHaveText('Connected');
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(true);
 await expect(page.locator('#background-image')).toBeVisible();
 await page.emulateMedia({reducedMotion:'no-preference'});
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(false);
 await page.reload();await expect(page.locator('#status')).toHaveText('Connected');
 await expect(page.locator('#background-video')).toBeVisible();
 await page.evaluate(()=>{document.querySelector('#background-video').dispatchEvent(new Event('error'));});
 await expect(page.locator('#video-status')).toHaveText('Video playback is unavailable. The still background remains visible.');
 await expect(page.locator('#background-video')).toBeHidden();await expect(page.locator('#background-image')).toBeVisible();
 await openVisual(page);await clickControl(page,'Remove background');
 await expect(screen(page)).not.toContainText('Background darkness:');
});


test('disconnect pauses without persisting pause and reconnect reuses an eligible selected player',async({page})=>{
 await page.addInitScript(()=>{
  const NativeWebSocket=WebSocket;
  window.WebSocket=class extends NativeWebSocket {constructor(url,...args){super(url,...args);if(new URL(url).pathname==='/ws')window.videoTestSocket=this;}};
 });
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');await selectVideo(page);
 await page.locator('#background-video').evaluate(video=>{video.dataset.identity='connection-owned';});
 await page.evaluate(()=>window.videoTestSocket.close());
 await expect(page.locator('#reconnect')).toBeVisible();
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(true);
 await expect(page.locator('#background-video')).toBeHidden();await expect(page.locator('#background-image')).toBeVisible();
 expect((await (await page.request.get('/visual')).json()).userPaused).toBe(false);
 await page.locator('#reconnect').click();await expect(page.locator('#status')).toHaveText('Connected');
 await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.paused)).toBe(false);
 await expect(page.locator('#background-video')).toBeVisible();
 expect(await page.locator('#background-video').getAttribute('data-identity')).toBe('connection-owned');
 await openVisual(page);await clickControl(page,'Remove background');
 await expect(screen(page)).not.toContainText('Background darkness:');
});


test('rejected play uses the poster and fixed feedback without retrying or reconnecting',async({page,baseURL})=>{
 const response=await page.request.post('/visual/video',{headers:{Origin:new URL(baseURL).origin,'X-Background-Settings':JSON.stringify({darkness:70,framing:{fit:'fill',zoom:100,positionX:5000,positionY:5000},speedPercent:100,userPaused:false})},data:videoFixture.buffer});
 expect(response.ok()).toBe(true);
 await page.addInitScript(()=>{window.videoPlayAttempts=0;HTMLMediaElement.prototype.play=function(){window.videoPlayAttempts++;return Promise.reject(new DOMException('synthetic blocked playback','NotAllowedError'));};});
 let sockets=0;page.on('websocket',()=>sockets++);
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 const socketsAtStart=sockets;
 await expect(page.locator('#video-status')).toHaveText('Video playback is unavailable. The still background remains visible.');
 await expect(page.locator('#background-image')).toBeVisible();await expect(page.locator('#background-video')).toBeHidden();
 await page.keyboard.press('Tab');await page.keyboard.press('ArrowLeft');await page.keyboard.press('ArrowRight');
 expect(await page.evaluate(()=>window.videoPlayAttempts)).toBe(1);expect(sockets).toBe(socketsAtStart);
 expect((await (await page.request.get('/visual')).json()).userPaused).toBe(false);
 await openVisual(page);await clickControl(page,'Remove background');
 await expect(screen(page)).not.toContainText('Background darkness:');
});

test('poster colors match the presented SDR frame within JPEG tolerance',async({page})=>{
 await page.goto('/');await expect(page.locator('#status')).toHaveText('Connected');
 await openVisual(page);await chooseImage(page,{name:'synthetic-colors.mp4',mimeType:'video/mp4',buffer:readFileSync(new URL('./fixtures/visual-video-colors.mp4',import.meta.url))});
 await clickControl(page,'Apply');await expect(screen(page)).not.toContainText('Background darkness:');await expect(page.locator('#background-video')).toBeVisible();
 const samples=await page.evaluate(async()=>{
  const video=document.querySelector('#background-video');video.pause();
  // Every frame has the same uniform bands; sampling avoids JPEG edge loss and seek timing.
  const canvas=document.createElement('canvas');canvas.width=video.videoWidth;canvas.height=video.videoHeight;
  const context=canvas.getContext('2d');
  const points=[40,120,200,280];
  context.drawImage(video,0,0);const frame=points.map(x=>[...context.getImageData(x,48,1,1).data]);
  context.drawImage(document.querySelector('#background-image'),0,0);const poster=points.map(x=>[...context.getImageData(x,48,1,1).data]);
  return {frame,poster};
 });
 for(let pixel=0;pixel<samples.frame.length;pixel++)for(let channel=0;channel<3;channel++)expect(Math.abs(samples.frame[pixel][channel]-samples.poster[pixel][channel])).toBeLessThanOrEqual(8);
 await openVisual(page);await clickControl(page,'Remove background');
 await expect(screen(page)).not.toContainText('Background darkness:');
});
