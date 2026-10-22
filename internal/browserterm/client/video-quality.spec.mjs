import {test,expect} from '@playwright/test';
import {readFileSync,mkdtempSync,rmSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {openVisual,clickControl,chooseImage,screen,setVideoQuality} from './visual-helpers.mjs';
import {fakeMicrophone} from './capture-fixture.mjs';
const video=readFileSync(new URL('./fixtures/visual-video.mp4',import.meta.url));
let hdVideo,fixtureDirectory;
test.beforeAll(()=>{
  fixtureDirectory=mkdtempSync(join(tmpdir(),'thoughts-quality-browser-'));
  const path=join(fixtureDirectory,'1080p60.mp4');
  execFileSync('ffmpeg',['-v','error','-f','lavfi','-i','testsrc2=size=1920x1080:rate=60:duration=1.25','-c:v','libx264','-threads','1','-preset','ultrafast','-pix_fmt','yuv420p','-color_primaries','bt709','-color_trc','bt709','-colorspace','bt709','-color_range','tv','-movflags','+faststart','-y',path]);
  hdVideo=readFileSync(path);
});
test.afterAll(()=>{if(fixtureDirectory)rmSync(fixtureDirectory,{recursive:true,force:true});});
const framing={fit:'fill',zoom:100,positionX:5000,positionY:5000};

test.beforeEach(async({page,baseURL})=>{
  // Reset before opening the page: its initial background restore also reads
  // Visual settings and legitimately excludes a concurrent settings mutation.
  const response=await page.request.put('/visual/settings',{headers:{Origin:new URL(baseURL).origin},data:{darkness:70,framing,speedPercent:100,userPaused:false,videoQuality:'efficient'}});
  await expect(response).toBeOK();
  await page.goto('/');
  await expect(screen(page)).toContainText('Events');
});

for(const tier of ['efficient','high','maximum'])test(`video quality ${tier} persists the acknowledged stream and preference`,async({page})=>{
  await openVisual(page);
  await setVideoQuality(page,tier);
  const reply=page.waitForResponse(r=>r.url().endsWith('/visual/video/preview')&&r.request().method()==='POST');
  await chooseImage(page,{name:'quality.mp4',mimeType:'video/mp4',buffer:hdVideo});
  const prepared=await(await reply).json();
  expect(prepared.videoQuality).toBe(tier);
  expect(prepared.width).toBe(tier==='efficient'?1280:1920);
  expect(prepared.height).toBe(tier==='efficient'?720:1080);
  expect(prepared.frameRate).toEqual({numerator:tier==='maximum'?60:30,denominator:1});
  await page.screenshot({path:`/tmp/video-quality-${test.info().project.name}-${tier}.png`});
  await clickControl(page,'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  expect(await(await page.request.get('/visual')).json()).toMatchObject({videoQuality:tier,background:true});
  await expect.poll(()=>page.locator('#background-video').evaluate(v=>[v.videoWidth,v.videoHeight])).toEqual([prepared.width,prepared.height]);
  await expect.poll(()=>page.locator('#background-video').evaluate(video=>video.currentTime)).toBeGreaterThan(.25);
  await page.reload();
  await expect(screen(page)).toContainText('Events');
  await openVisual(page);
  await expect(screen(page)).toContainText(`Video quality: ${{efficient:'Efficient',high:'High quality',maximum:'Maximum'}[tier]}`);
  await clickControl(page,'Remove background');
  await expect(screen(page)).not.toContainText('Background darkness:');
});

test('unpreviewed quality changes retain accepted quality and bytes through Apply',async({page})=>{
  await openVisual(page);
  const reply=page.waitForResponse(r=>r.url().endsWith('/visual/video/preview')&&r.request().method()==='POST');
  await chooseImage(page,{name:'accepted.mp4',mimeType:'video/mp4',buffer:video});
  const response=await reply,prepared=await response.json();
  const headers={'X-Visual-Editor':(await response.request().allHeaders())['x-visual-editor']};
  const acceptedVideo=await(await page.request.get(`/visual/video/preview?id=${prepared.previewID}&part=video`,{headers})).body();
  const acceptedPoster=await(await page.request.get(`/visual/video/preview?id=${prepared.previewID}&part=poster`,{headers})).body();
  await clickControl(page,'Edit loop');
  await expect(screen(page)).toContainText('Source / Start:');
  await setVideoQuality(page,'maximum');
  await expect(screen(page)).toContainText('Previewed: Efficient');
  await expect(screen(page)).toContainText('Preview loop to use Maximum');
  await clickControl(page,'Done — use previewed loop');
  await expect(screen(page)).toContainText('Background darkness:');
  await page.setViewportSize({width:650,height:600});
  await expect(screen(page)).toContainText('Previewed: Efficient');
  await expect.poll(() => screen(page).locator('div').filter({hasText:'Video quality: Maximum'}).first().evaluate(row => {
    const range=document.createRange();range.selectNodeContents(row);return range.getBoundingClientRect().right;
  })).toBeLessThanOrEqual(650);
  await page.screenshot({path:`/tmp/video-quality-${test.info().project.name}-draft.png`});
  await clickControl(page,'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  expect(await(await page.request.get('/visual')).json()).toMatchObject({videoQuality:'efficient'});
  expect(await(await page.request.get('/visual/background')).body()).toEqual(acceptedVideo);
  expect(await(await page.request.get('/visual/poster')).body()).toEqual(acceptedPoster);
});

test('a new explicit quality preview replaces the candidate while settings saves never encode',async({page})=>{
  await openVisual(page);
  await chooseImage(page,{name:'quality-change.mp4',mimeType:'video/mp4',buffer:video});
  await clickControl(page,'Edit loop');
  await expect(screen(page)).toContainText('Source / Start:');
  await setVideoQuality(page,'maximum');
  const reply=page.waitForResponse(r=>r.url().endsWith('/visual/video/preview')&&r.request().method()==='POST');
  await clickControl(page,'Preview loop');
  expect((await(await reply).json()).videoQuality).toBe('maximum');
  await expect(screen(page)).toContainText('Previewed: Maximum');
  await clickControl(page,'Done — use previewed loop');
  await expect(screen(page)).toContainText('Background darkness:');
  await clickControl(page,'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  const selected=await(await page.request.get('/visual')).json();
  expect(selected.videoQuality).toBe('maximum');
  let preparations=0;page.on('request',r=>{if(r.url().includes('/visual/video')&&r.method()==='POST')preparations++;});
  await openVisual(page);
  await setVideoQuality(page,'high');
  await clickControl(page,'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  expect(await(await page.request.get('/visual')).json()).toMatchObject({backgroundID:selected.backgroundID,videoQuality:'high'});
  expect(preparations).toBe(0);
});

test('Maximum preparation preserves the recording, draft and terminal connection', async ({page}) => {
  let terminalSockets = 0;
  page.on('websocket', socket => { if (new URL(socket.url()).pathname === '/ws') terminalSockets++; });
  await fakeMicrophone(page);
  await page.reload();
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Events');
  await page.keyboard.press('t');
  await expect(screen(page)).toContainText('Create thought');
  await page.keyboard.type('quality import voice draft');
  await page.keyboard.press('F8');
  await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  await page.evaluate(() => window.voiceProbe.grant(0));
  await expect(page.locator('#status')).toHaveText('Transcribing…');
  const activeSockets = terminalSockets;
  await openVisual(page);
  await setVideoQuality(page, 'maximum');
  // A real picker uses a local file. Injecting its megabytes through the driver
  // can block Chromium's main thread and exhaust the bounded audio-worklet credits.
  await chooseImage(page, {mimeType:'video/mp4', path:join(fixtureDirectory,'1080p60.mp4')});
  await expect(screen(page)).toContainText('Previewed: Maximum');
  await clickControl(page, 'Apply');
  await expect(screen(page)).not.toContainText('Background darkness:');
  await expect(screen(page)).toContainText('quality import voice draft');
  expect(await page.evaluate(() => window.voiceProbe.requests)).toBe(1);
  expect(await page.evaluate(() => window.voiceProbe.tracksStopped)).toBe(0);
  expect(terminalSockets).toBe(activeSockets);
  await expect(page.locator('#status')).toHaveText('Transcribing…');
  await expect.poll(() => page.locator('#background-video').evaluate(video => video.videoWidth)).toBe(1920);
  await page.keyboard.press('F8');
  await expect(screen(page)).toContainText('F8: Record');
});
