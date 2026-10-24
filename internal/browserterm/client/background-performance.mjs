// Opt-in local probe: imports/removes synthetic backgrounds on a disposable demo.
// Run revisions separately; do not run builds/tests alongside measurements.
import {chromium, expect} from '@playwright/test';
import {fakeMicrophone} from './capture-fixture.mjs';
import {openVisual, setDarkness, screen, setVideoQuality} from './visual-helpers.mjs';
import {readFileSync, writeFileSync, createReadStream, statSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {availableParallelism} from 'node:os';
import {createHash} from 'node:crypto';
import {Worker} from 'node:worker_threads';
import {deflateSync} from 'node:zlib';
import {measureVideoInteractions} from './video-interaction-performance.mjs';

if (process.env.THOUGHTS_BACKGROUND_PERF_DISPOSABLE !== '1') throw new Error('Use only a disposable seeded demo; set THOUGHTS_BACKGROUND_PERF_DISPOSABLE=1.');
const [url, appPID, output] = process.argv.slice(2);
if (!url || new URL(url).hostname !== '127.0.0.1' || !/^\d+$/.test(appPID || '') || !output?.startsWith('/tmp/')) throw new Error('Usage: node background-performance.mjs http://127.0.0.1:PORT APP_PID /tmp/results.json');
const ticks = Number(execFileSync('getconf',['CLK_TCK'],{encoding:'utf8'}));
const count = Number(process.env.THOUGHTS_PERF_SAMPLES || 5), rows = [];
if (!Number.isInteger(count) || count < 1 || count > 5) throw new Error('Samples must be between 1 and 5.');
const fixtureRunID = Date.now(); // Avoid native file-picker deduplication between separate probe runs.
// Privacy follow-ups measure only one-time import/activation; avoid repeating idle/navigation matrices.
const importOnly = process.env.THOUGHTS_PERF_IMPORT_ONLY === '1';
const videoFixturePath = process.env.THOUGHTS_PERF_VIDEO_FIXTURE;
const longSourceStudy = process.env.THOUGHTS_PERF_LONG_SOURCE === '1';
const qualityStudy = process.env.THOUGHTS_PERF_QUALITY_STUDY === '1';
const qualityTier=process.env.THOUGHTS_PERF_VIDEO_QUALITY || 'efficient';
if(!['efficient','high','maximum'].includes(qualityTier))throw new Error('Unknown performance quality tier');
// Idle-only follow-ups can retain their own sampling count.
const idleSamples = Number(process.env.THOUGHTS_PERF_IDLE_SAMPLES || 5);
if (!Number.isInteger(idleSamples) || idleSamples < 1 || idleSamples > 5) throw new Error('Idle samples must be between 1 and 5.');
const median = values => [...values].sort((a,b)=>a-b)[Math.floor(values.length/2)];
const percentile95 = values => [...values].sort((a,b)=>a-b)[Math.ceil(values.length*.95)-1];
// Generate identical synthetic PNGs without Brave's randomized canvas readback.
// Fixture construction and compression finish before the measured browser starts.
function pngChunk(type, body) {
  const chunk = Buffer.alloc(body.length + 12);
  chunk.writeUInt32BE(body.length);
  chunk.write(type, 4);
  body.copy(chunk, 8);
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
function syntheticPNG(width, height) {
  // RGB8 uses three bytes per pixel; each PNG row begins with filter byte 0
  // (None), supplied by the zero-initialized buffer. Fixture colors are decorative.
  const stride = width * 3 + 1;
  const pixels = Buffer.alloc(stride * height);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const offset = y * stride + 1 + x * 3;
      if (x % 64 < 12 && y % 48 < 12) {
        pixels[offset] = 172;
        pixels[offset + 1] = 69;
        pixels[offset + 2] = 141;
      } else {
        pixels[offset] = 36 + Math.floor(196 * x / width);
        pixels[offset + 1] = 120 + Math.floor(63 * y / height);
        pixels[offset + 2] = 174 - Math.floor(90 * x / width);
      }
    }
  }
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width);
  header.writeUInt32BE(height, 4);
  // IHDR: width/height uint32, depth 8, color type 2 (RGB), default methods 0.
  header[8] = 8;
  header[9] = 2;
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    pngChunk('IHDR', header), pngChunk('IDAT', deflateSync(pixels)), pngChunk('IEND', Buffer.alloc(0))
  ]);
}
const fixtures = (longSourceStudy ? [] : videoFixturePath ? [[1200,850]] : importOnly ? [[1200,850],[6000,4000]] : [[1267,719],[91,4093],[4093,91],[6000,4000]]).map(([width,height]) => ({width,height,buffer:syntheticPNG(width,height)}));
const browser = await chromium.launch({headless:false, executablePath:process.env.THOUGHTS_PERF_BROWSER || '/usr/bin/brave-browser'});
try {
  const page = await browser.newPage({viewport:null});
  const cdp = await page.context().newCDPSession(page);
  const browserCDP = await browser.newBrowserCDPSession();
  if(process.env.THOUGHTS_PERF_RECORDING==='1' || qualityStudy)await fakeMicrophone(page,.25);
  if (process.env.THOUGHTS_PERF_INTERACTIONS === '1') await page.addInitScript(() => {
    // Test-only arrival stamp: split application/transport waiting from xterm's
    // queued rendering without adding instrumentation to the shipped adapter.
    const NativeWebSocket = WebSocket;
    window.WebSocket = class extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        if (new URL(args[0], location.href).pathname === '/ws') {
          window.perfTerminalConnections = (window.perfTerminalConnections || 0) + 1;
        }
        this.addEventListener('message', event => {
          if (event.data instanceof ArrayBuffer && new Uint8Array(event.data)[0] === 49) window.lastTerminalArrival = performance.now();
        });
      }
    };
  });
  await page.route('**/static/visual.js', async route => {
    const response = await route.fetch();
    await route.fulfill({response,body:await response.text()+`;window.activationTimes=[];const measuredActivate=ThoughtsVisual.prototype.activate;ThoughtsVisual.prototype.activate=async function(...args){window.backgroundProbe=this;const start=performance.now();const result=await measuredActivate.apply(this,args);window.activationTimes.push(performance.now()-start);return result;};`});
  });
  await page.goto(url); await page.bringToFront(); await expect(page.locator('#status')).toHaveText('Connected');
  // A demo without the optional native speech runtime opens Models on first
  // connection. Dismiss that prompt before exercising timeline/Visual controls.
  if (process.env.THOUGHTS_PERF_DISMISS_SPEECH_SETUP === '1') {
    await expect(screen(page)).toContainText('Not now');
    await page.locator('.xterm-helper-textarea').focus();
    await page.keyboard.press('Escape');
    await expect(screen(page)).not.toContainText('Not now');
  }
  const {windowId} = await cdp.send('Browser.getWindowForTarget');
  const fullscreen = async () => { await cdp.send('Browser.setWindowBounds',{windowId,bounds:{windowState:'fullscreen'}}); await page.waitForTimeout(500); };
  await fullscreen();
  const metadata = {videoQuality:qualityTier,qualityStudy,previousDay:process.env.THOUGHTS_PERF_PREVIOUS_DAY==='1',revision:process.env.THOUGHTS_PERF_REVISION || execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),logicalCPUs:availableParallelism(),contentionWorkers:Math.min(4,Math.max(1,Math.floor(availableParallelism()/2))),browser:await browser.version(),go:execFileSync('go',['version'],{encoding:'utf8'}).trim(),system:await browserCDP.send('SystemInfo.getInfo'),viewport:await page.evaluate(()=>[innerWidth,innerHeight,devicePixelRatio]),count,idleSamples,importOnly,appPID};
  metadata.decoderProperties=[];
  await cdp.send('Media.enable');
  cdp.on('Media.playerPropertiesChanged',event=>{
    const properties=event.properties.filter(property=>/decoder/i.test(property.name));
    if(properties.length)metadata.decoderProperties.push(properties);
  });
  // Keyboard focus avoids pointer coordinates racing the desktop window manager.
  const clickControl = async (page, label) => {
    await page.locator('.xterm-helper-textarea').focus();
    for (let step = 0; step < 16; step++) {
      const selected = page.locator('.xterm-bg-62').filter({hasText: new RegExp('^' + label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '$')});
      if (await selected.count()) {
        await page.keyboard.press('Enter');
        if (label === 'Pause video' || label === 'Play video') {
          // Playback controls acknowledge asynchronously. Wait for that reply
          // before sending another key, which the busy editor would ignore.
          await page.waitForFunction(paused => {
            const state = window.backgroundProbe?.state;
            return state?.request === '' && state.userPaused === paused;
          }, label === 'Pause video');
        }
        return;
      }
      const previous = await page.locator('.xterm-bg-62').allTextContents();
      await page.keyboard.press('Tab');
      await expect.poll(() => page.locator('.xterm-bg-62').allTextContents()).not.toEqual(previous);
    }
    throw new Error('Missing focused Visual control: ' + label);
  };
  const chooseImage = async (page, fixture) => {
    if(fixture.mimeType?.startsWith('video/'))await setVideoQuality(page,qualityTier);
    const chooser = page.waitForEvent('filechooser');
    await clickControl(page, 'Choose image or video');
    await (await chooser).setFiles(fixture.path || fixture);
    if(fixture.mimeType?.startsWith('video/')) {
      await expect(screen(page)).toContainText('Set cut points, then Preview loop.',{timeout:15000});
      const preparedReply=page.waitForResponse(response=>response.url().endsWith('/visual/video/preview')&&response.request().method()==='POST',{timeout:125000});
      await clickControl(page,'Preview loop');
      await expect(screen(page)).toContainText('Preview ready. Done uses this loop.',{timeout:120000});
      const {videoQuality,width,height,frameRate,frameCount,durationMS,trim}=await(await preparedReply).json();
      add({phase:'prepared-stream',videoQuality,width,height,frameRate,frameCount,durationMS,trim});
      await clickControl(page,'Done — use previewed loop');
      await expect(screen(page)).toContainText('Background darkness:');
    } else await expect(screen(page)).toContainText('Preview only. Apply to save.', {timeout: 15000});
    await expect(page.locator('#background-preview-wrap')).toBeVisible();
  };
  const usage = async () => {
    const {processInfo} = await browserCDP.send('SystemInfo.getProcessInfo');
    let browserCPU=0,browserRSS=0,browserPSS=0;
    for (const process of processInfo) {
      browserCPU+=process.cpuTime;
      try { browserRSS+=Number(readFileSync(`/proc/${process.id}/status`,'utf8').match(/VmRSS:\s+(\d+)/)[1]); } catch {}
      try { browserPSS+=Number(readFileSync(`/proc/${process.id}/smaps_rollup`,'utf8').match(/^Pss:\s+(\d+)/m)[1]); } catch {}
    }
    // After stripping pid/(comm), indices 11/12 are Linux stat fields 14/15
    // (user/system CPU ticks). CLK_TCK converts them to CPU seconds;
    // /proc memory values are KiB and become MiB below.
    const stat=readFileSync(`/proc/${appPID}/stat`,'utf8').split(') ')[1].split(' ');
    const appCPU=(Number(stat[11])+Number(stat[12]))/ticks;
    const appRSS=Number(readFileSync(`/proc/${appPID}/status`,'utf8').match(/VmRSS:\s+(\d+)/)[1]);
    return {browserCPU,appCPU,browserRSS:browserRSS/1024,browserPSS:browserPSS/1024,appRSS:appRSS/1024,processCPU:processInfo.map(({id,type,cpuTime}) => ({id,type,cpuTime}))};
  };
  const add = row => {rows.push(row);writeFileSync(output,JSON.stringify({metadata,rows},null,2));console.log(JSON.stringify(row));};
  const idle = async image => {
    await page.waitForTimeout(3000);
    for(let run=0;run<idleSamples;run++) {
      const a=await usage(),start=performance.now();await page.waitForTimeout(30000);const b=await usage();const seconds=(performance.now()-start)/1000;
      const quality=await page.evaluate(()=>{const video=document.querySelector('#background-video');if(!video)return null;const quality=video.getVideoPlaybackQuality();return {paused:video.paused,currentTime:video.currentTime,frames:quality.totalVideoFrames,dropped:quality.droppedVideoFrames,visibility:document.visibilityState};});
      add({phase:'idle',image,run,quality,browserCPUPercent:100*(b.browserCPU-a.browserCPU)/seconds,appCPUPercent:100*(b.appCPU-a.appCPU)/seconds,...b});
    }
  };
  // Measure keydown -> observed terminal row mutation, excluding automation delivery.
  const keyLatency = async key => {
    await page.locator('.xterm-helper-textarea').focus();
    await page.evaluate(() => {
      window.keyLatency=new Promise(resolve=>document.addEventListener('keydown',()=>{
        const start=performance.now();const observer=new MutationObserver(()=>{observer.disconnect();resolve(performance.now()-start);});
        observer.observe(document.querySelector('.xterm-rows'),{subtree:true,childList:true,attributes:true,characterData:true});
      },{once:true,capture:true}));
    });
    await page.keyboard.press(key);
    return page.evaluate(()=>Promise.race([window.keyLatency,new Promise(resolve=>setTimeout(()=>resolve(null),2000))]));
  };
  const navigation = async (image,contention=false) => {
    const workers=contention?Array.from({length:Math.min(4,Math.max(1,Math.floor(availableParallelism()/2)))},()=>new Worker('const end=Date.now()+60000;while(Date.now()<end){Math.sqrt(Date.now());}',{eval:true})):[];
    try {
      if(contention)await page.waitForTimeout(1000);
      for(let run=0;run<count;run++) {
        const before=await usage(),values=[];
        for(let i=0;i<10;i++) {values.push(await keyLatency('ArrowLeft'));await expect(page.locator('.xterm-bg-62').filter({hasText:/^Subjects$/})).toBeVisible();values.push(await keyLatency('ArrowRight'));await expect(page.locator('.xterm-bg-62').filter({hasText:/^Options$/})).toBeVisible();}
        const after=await usage();add({phase:'navigation',image,contention,run,medianMs:median(values.filter(v=>v!==null)),p95Ms:percentile95(values.filter(v=>v!==null)),timeouts:values.filter(v=>v===null).length,browserCPUMs:1000*(after.browserCPU-before.browserCPU),appCPUMs:1000*(after.appCPU-before.appCPU)});
      }
    } finally {await Promise.all(workers.map(worker=>worker.terminate()));}
  };
  if (process.env.THOUGHTS_PERF_INTERACTIONS === '1') {
    if (!videoFixturePath) throw new Error('Interaction mode requires THOUGHTS_PERF_VIDEO_FIXTURE.');
    await measureVideoInteractions({page, cdp, url, videoFixturePath, output, add, usage, count, qualityStudy, openVisual, chooseImage, clickControl});
  } else {
  await openVisual(page);await clickControl(page,'Remove background');await expect(screen(page)).not.toContainText('Background darkness:');
  if(!importOnly&&!videoFixturePath)await idle('opaque');
  await page.bringToFront();await page.locator('.xterm-helper-textarea').focus();await page.keyboard.press('Tab');await expect(screen(page)).toContainText('Tab: mindset • Q: events');await page.keyboard.press('ArrowRight');await page.keyboard.press('Enter');await expect(screen(page)).toContainText('Background darkness:');await expect(screen(page)).not.toContainText('Loading visual');
  for(const fixture of fixtures) {
    const image=`${fixture.width}x${fixture.height}`;
    for(let run=0;run<count;run++) {
      if(run || fixture!==fixtures[0])await openVisual(page);
      const initial=await usage();let peakBrowserRSS=initial.browserRSS,peakAppRSS=initial.appRSS;
      const timer=setInterval(()=>usage().then(v=>{peakBrowserRSS=Math.max(peakBrowserRSS,v.browserRSS);peakAppRSS=Math.max(peakAppRSS,v.appRSS);}).catch(()=>{}),100);
      try {
      const start=performance.now();await chooseImage(page,{name:`synthetic-${fixtureRunID}-${image}-${run}.png`,mimeType:'image/png',buffer:fixture.buffer});
      const previewMs=performance.now()-start;
      if((await screen(page).textContent()).includes('Background fit: Fit'))await clickControl(page,'Background fit: Fit');
      await setDarkness(page,30);
      await page.evaluate(()=>{window.activationTimes=[];});
      const applyStart=performance.now();
        await clickControl(page,'Apply');await expect(screen(page)).not.toContainText('Background darkness:');
        const applyMs=performance.now()-applyStart;
        await page.keyboard.press('ArrowLeft');await expect(page.locator('.xterm-bg-62').filter({hasText:/^Subjects$/})).toBeVisible();await page.keyboard.press('ArrowRight');await expect(page.locator('.xterm-bg-62').filter({hasText:/^Options$/})).toBeVisible();
        const end=await usage();
        add({phase:'import',image,run,viewport:await page.evaluate(()=>[innerWidth,innerHeight]),bytes:fixture.buffer.length,sha256:createHash('sha256').update(fixture.buffer).digest('hex'),previewMs,applyMs,returnAndSelectionMs:performance.now()-applyStart,activationMs:await page.evaluate(()=>window.activationTimes.at(-1)),stored:await page.locator('#background-image').evaluate(i=>[i.naturalWidth,i.naturalHeight]),browserCPUMs:1000*(end.browserCPU-initial.browserCPU),appCPUMs:1000*(end.appCPU-initial.appCPU),peakBrowserRSS,peakAppRSS,...end});
      } finally {clearInterval(timer);}
    }
    if(!importOnly)await navigation(image);
    if(!importOnly && !videoFixturePath && (fixture===fixtures[0] || fixture===fixtures.at(-1)))await idle(image);
    if(!importOnly && !videoFixturePath && fixture===fixtures.at(-1))await navigation(image,true);
  }
  if(videoFixturePath && longSourceStudy) {
    const hash=createHash('sha256');for await(const chunk of createReadStream(videoFixturePath))hash.update(chunk);
    add({phase:'long-source-fixture',bytes:statSync(videoFixturePath).size,sha256:hash.digest('hex')});
    for(let run=0;run<count;run++) {
      await openVisual(page);await setVideoQuality(page,qualityTier);
      const initial=await usage();let peakBrowserRSS=initial.browserRSS,peakBrowserPSS=initial.browserPSS,peakAppRSS=initial.appRSS;
      const timer=setInterval(async()=>{const current=await usage();peakBrowserRSS=Math.max(peakBrowserRSS,current.browserRSS);peakBrowserPSS=Math.max(peakBrowserPSS,current.browserPSS);peakAppRSS=Math.max(peakAppRSS,current.appRSS);},200);
      try {
        const chooser=page.waitForEvent('filechooser');await clickControl(page,'Choose image or video');
        const selectionStart=performance.now();await(await chooser).setFiles(videoFixturePath);
        await expect(screen(page)).toContainText('Set cut points, then Preview loop.');
        await expect(page.locator('#background-preview-video')).toBeVisible();
        const selectionMs=performance.now()-selectionStart;
        const sourceInfo=await page.locator('#background-preview-video').evaluate(v=>({duration:v.duration,width:v.videoWidth,height:v.videoHeight}));
        const seekMs=[];
        for(const position of [sourceInfo.duration/2,sourceInfo.duration-10,sourceInfo.duration/4]) {
          const start=performance.now();await page.keyboard.press('Enter');await page.keyboard.press('Control+u');
          const ms=Math.floor(position*1000),time=`${String(Math.floor(ms/3600000)).padStart(2,'0')}:${String(Math.floor(ms/60000)%60).padStart(2,'0')}:${String(Math.floor(ms/1000)%60).padStart(2,'0')}.${String(ms%1000).padStart(3,'0')}`;
          await page.keyboard.insertText(time);await page.keyboard.press('Enter');
          await expect.poll(()=>page.locator('#background-preview-video').evaluate((v,target)=>!v.seeking&&Math.abs(v.currentTime-target/1000)<.001,ms)).toBe(true);
          seekMs.push(performance.now()-start);
        }
        // A final late cut is measured separately from exploratory seeking.
        await page.keyboard.press('End');for(let i=0;i<9;i++)await page.keyboard.press('Shift+ArrowLeft');
        const stageReply=page.waitForResponse(r=>new URL(r.url()).pathname==='/visual/video/source'&&r.request().method()==='POST',{timeout:1325000});
        const prepareReply=page.waitForResponse(r=>new URL(r.url()).pathname==='/visual/video/preview'&&r.request().method()==='POST',{timeout:1450000});
        let transferredAt;
        await page.evaluate(()=>{
          window.longSourceTransferDone=undefined;
          const send=XMLHttpRequest.prototype.send;
          XMLHttpRequest.prototype.send=function(body){this.upload.addEventListener('load',()=>{window.longSourceTransferDone=performance.now();},{once:true});return send.call(this,body);};
        });
        const browserStart=await page.evaluate(()=>performance.now());const prepareStart=performance.now();await clickControl(page,'Preview loop');
        await stageReply;const stagingMs=performance.now()-prepareStart;
        transferredAt=await page.evaluate(()=>window.longSourceTransferDone);
        const response=await prepareReply;const prepared=await response.json(),preparationMs=performance.now()-prepareStart-stagingMs;
        const readyStart=performance.now();await expect(screen(page)).toContainText('Preview ready. Done uses this loop.',{timeout:125000});const previewLoadMs=performance.now()-readyStart;
        await clickControl(page,'Done — use previewed loop');const applyStart=performance.now();await clickControl(page,'Apply');await expect(screen(page)).not.toContainText('Background darkness:');
        const end=await usage();
        add({phase:'long-source-import',run,sourceInfo,selectionMs,seekMs,transferMs:transferredAt-browserStart,stagingMs,validationResponseMs:stagingMs-(transferredAt-browserStart),preparationMs,previewLoadMs,applyMs:performance.now()-applyStart,prepared,peakBrowserRSS,peakBrowserPSS,peakAppRSS,browserCPUMs:1000*(end.browserCPU-initial.browserCPU),appCPUMs:1000*(end.appCPU-initial.appCPU),...end});
      } finally {clearInterval(timer);}
    }
  }
  if(videoFixturePath && !longSourceStudy) {
    const hash=createHash('sha256');for await(const chunk of createReadStream(videoFixturePath))hash.update(chunk);
    add({phase:'video-fixture',bytes:statSync(videoFixturePath).size,sha256:hash.digest('hex')});
    const events=async mode=>{
      // Tab leaves the entity picker; day keys then exercise actual Events.
      await page.keyboard.press('Tab');
      for(let run=0;run<count;run++) {
        const values=[];
        for(let i=0;i<10;i++){values.push(await keyLatency('ArrowLeft'));values.push(await keyLatency('ArrowRight'));}
        add({phase:'video-events',mode,run,p95Ms:percentile95(values.filter(value=>value!==null)),medianMs:median(values.filter(value=>value!==null)),timeouts:values.filter(value=>value===null).length});
      }
      await page.keyboard.press('Tab');
    };
    if(!importOnly){await idle('static-video-baseline');await events('static');}
    for(let run=0;run<count;run++) {
      await openVisual(page);
      const start=performance.now();
      await chooseImage(page,{name:`synthetic-video-${fixtureRunID}-${run}.mp4`,mimeType:'video/mp4',path:videoFixturePath});
      const previewMs=performance.now()-start;
      // The same cut reuses acknowledged native files, while the browser may
      // re-show its already decoded player. This must not launch FFmpeg.
      await clickControl(page,'Edit loop');
      await expect(screen(page)).toContainText('Previewed:');
      const reuseStart=performance.now();
      const reused=page.waitForResponse(response=>new URL(response.url()).pathname==='/visual/video/preview'&&response.request().method()==='POST');
      await clickControl(page,'Preview loop');await reused;
      await page.waitForFunction(()=>window.backgroundProbe.state.request==='');
      await expect(screen(page)).toContainText('Preview ready. Done uses this loop.');
      const reuseMs=performance.now()-reuseStart;
      await clickControl(page,'Done — use previewed loop');
      await expect(screen(page)).toContainText('Background darkness:');
      const applyStart=performance.now();await clickControl(page,'Apply');
      await expect(screen(page)).not.toContainText('Background darkness:',{timeout:120000});
      await expect(page.locator('#background-video')).toBeVisible();
      add({phase:'video-import',run,previewMs,reuseMs,applyMs:performance.now()-applyStart,activationMs:await page.evaluate(()=>window.activationTimes.at(-1)),...await usage()});
    }
    if(!importOnly) {
    for(const percent of [100,50,25]) {
      await openVisual(page);await clickControl(page,`Playback speed: ${percent===100?100:percent===50?100:50}%`);
      await page.keyboard.press('Home');for(let value=25;value<percent;value+=5)await page.keyboard.press('ArrowRight');
      await clickControl(page,'Apply');await expect(screen(page)).not.toContainText('Background darkness:');
      await idle(`video-${percent}`);
      if(percent===100) {
        await events('video');
        if(process.env.THOUGHTS_PERF_RECORDING==='1') {
          await page.keyboard.press('F8');await expect(screen(page)).toContainText('Create thought');
          await page.keyboard.press('F8');await expect.poll(()=>page.evaluate(()=>window.voiceProbe.requests)).toBe(1);
          await page.evaluate(()=>window.voiceProbe.grant(0));await expect(page.locator('#status')).toHaveText('Transcribing…');
          await idle('video-synthetic-recording');
          await page.keyboard.press('F8');await expect(screen(page)).toContainText('F8: Record');
          await page.keyboard.press('Escape');await page.keyboard.press('Tab');await page.keyboard.press('q');await page.keyboard.press('Tab');
          await expect(screen(page)).toContainText('Tab: mindset • Q: events');
        }
      }
    }
    await openVisual(page);await clickControl(page,'Pause video');await clickControl(page,'Apply');
    await expect(screen(page)).not.toContainText('Background darkness:');await idle('video-paused');
    await openVisual(page);await clickControl(page,'Play video');await clickControl(page,'Apply');
    await expect(screen(page)).not.toContainText('Background darkness:');
    await page.emulateMedia({reducedMotion:'reduce'});await idle('video-reduced-motion');
    await page.emulateMedia({reducedMotion:'no-preference'});
    // A second desktop window tests real visibility rather than dispatching a
    // fabricated event. Record document visibility so failed hiding is explicit.
    const other=await browser.newPage();await other.goto('about:blank');await other.bringToFront();
    await idle('video-hidden-attempt');await other.close();await page.bringToFront();
    // Some automated desktop setups keep inactive windows visible. Exercise
    // the real application listener with an explicitly labeled hidden state,
    // without describing that emulation as a successful OS/tab transition.
    await page.evaluate(()=>{Object.defineProperty(document,'hidden',{configurable:true,value:true});Object.defineProperty(document,'visibilityState',{configurable:true,value:'hidden'});document.dispatchEvent(new Event('visibilitychange'));});
    await idle('video-hidden-emulated');
    await page.evaluate(()=>{delete document.hidden;delete document.visibilityState;document.dispatchEvent(new Event('visibilitychange'));});
    }
  }
  if(!importOnly && !videoFixturePath) {
    // Keep the Events workload separate from the reported entity-picker artifact.
    await page.keyboard.press('Tab');
    for(let run=0;run<count;run++) {
      await page.keyboard.press('Home');await page.waitForTimeout(50);
      const values=[];for(let i=0;i<10;i++){values.push(await keyLatency('ArrowDown'));values.push(await keyLatency('ArrowUp'));}
      const days=[];for(let i=0;i<4;i++){days.push(await keyLatency('ArrowLeft'));days.push(await keyLatency('ArrowRight'));}
      add({phase:'events',run,scrollMedianMs:median(values.filter(v=>v!==null)),dayMedianMs:median(days.filter(v=>v!==null)),dayP95Ms:percentile95(days.filter(v=>v!==null))});
    }
    await page.keyboard.press('t');await expect(screen(page)).toContainText('Create thought');
    for(let run=0;run<count;run++){const values=[];for(const character of 'cpuheadroom')values.push(await keyLatency(character));add({phase:'typing',run,medianMs:median(values.filter(v=>v!==null)),p95Ms:percentile95(values.filter(v=>v!==null))});}
    await page.keyboard.press('Escape');
    const watchResize=()=>page.evaluate(()=>{window.resizeLatency=new Promise(resolve=>window.addEventListener('resize',()=>{const start=performance.now();const observer=new MutationObserver(()=>{observer.disconnect();resolve(performance.now()-start);});observer.observe(document.querySelector('.xterm-rows'),{subtree:true,childList:true,attributes:true});},{once:true}));});
    const resizeLatency=()=>page.evaluate(()=>Promise.race([window.resizeLatency,new Promise(resolve=>setTimeout(()=>resolve(null),2000))]));
    for(let run=0;run<count;run++){
      await watchResize();const start=performance.now();await cdp.send('Browser.setWindowBounds',{windowId,bounds:{windowState:'normal'}});await page.waitForTimeout(300);
      await page.screenshot({path:output.replace('.json',`-narrow-${run}.png`)});
      const narrow=await page.evaluate(()=>[innerWidth,innerHeight]);const narrowMs=await resizeLatency();await watchResize();await fullscreen();const desktopMs=await resizeLatency();
      add({phase:'resize',run,pairIncludingSettleMs:performance.now()-start,narrowMs,desktopMs,narrow,desktop:await page.evaluate(()=>[innerWidth,innerHeight])});
    }
  }
  await page.screenshot({path:output.replace('.json','-desktop.png')});
  await openVisual(page);await clickControl(page,'Remove background');await expect(screen(page)).not.toContainText('Background darkness:');
  }
} catch (error) {
  const page = browser.contexts()[0]?.pages()[0];
  if (page) {
    try {
      await page.screenshot({path:output.replace('.json','-error.png')});
      writeFileSync(output.replace('.json','-error.txt'),(await page.locator('.xterm-rows').textContent()).slice(0,16384));
    } catch { console.error('Probe capture unavailable.'); }
  }
  throw error;
} finally {await browser.close();}
