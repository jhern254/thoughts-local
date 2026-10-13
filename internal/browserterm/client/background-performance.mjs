// Opt-in local probe: imports/removes synthetic backgrounds on a disposable demo.
// Run revisions separately; do not run builds/tests alongside measurements.
import {chromium, expect} from '@playwright/test';
import {openVisual, chooseImage, clickControl, setDarkness, screen} from './visual-helpers.mjs';
import {readFileSync, writeFileSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {availableParallelism} from 'node:os';
import {createHash} from 'node:crypto';
import {Worker} from 'node:worker_threads';

if (process.env.THOUGHTS_BACKGROUND_PERF_DISPOSABLE !== '1') throw new Error('Use only a disposable seeded demo; set THOUGHTS_BACKGROUND_PERF_DISPOSABLE=1.');
const [url, appPID, output] = process.argv.slice(2);
if (!url || new URL(url).hostname !== '127.0.0.1' || !/^\d+$/.test(appPID || '') || !output?.startsWith('/tmp/')) throw new Error('Usage: node background-performance.mjs http://127.0.0.1:PORT APP_PID /tmp/results.json');
const ticks = Number(execFileSync('getconf',['CLK_TCK'],{encoding:'utf8'}));
const count = 5, rows = [];
const median = values => [...values].sort((a,b)=>a-b)[Math.floor(values.length/2)];
const percentile95 = values => [...values].sort((a,b)=>a-b)[Math.ceil(values.length*.95)-1];
// Generate fixtures in a separate browser, closed before CPU/memory sampling.
const fixtureBrowser = await chromium.launch({headless:true,executablePath:process.env.THOUGHTS_PERF_BROWSER || '/usr/bin/brave-browser'});
const fixturePage = await fixtureBrowser.newPage();
const fixtures=[];
  for(const [width,height] of [[1267,719],[91,4093],[4093,91],[6000,4000]]) {
    const encoded=await fixturePage.evaluate(([width,height])=>{
      const canvas=document.createElement('canvas');canvas.width=width;canvas.height=height;const ctx=canvas.getContext('2d');
      const gradient=ctx.createLinearGradient(0,0,width,height);gradient.addColorStop(0,'#2478ae');gradient.addColorStop(1,'#e8b754');ctx.fillStyle=gradient;ctx.fillRect(0,0,width,height);
      ctx.fillStyle='#ac458d';for(let y=0;y<height;y+=48)for(let x=0;x<width;x+=64)ctx.fillRect(x,y,12,12);
      return canvas.toDataURL('image/png').split(',')[1];
    },[width,height]);
    fixtures.push({width,height,buffer:Buffer.from(encoded,'base64')});
  }
await fixtureBrowser.close();
const browser = await chromium.launch({headless:false, executablePath:process.env.THOUGHTS_PERF_BROWSER || '/usr/bin/brave-browser'});
try {
  const page = await browser.newPage({viewport:null});
  const cdp = await page.context().newCDPSession(page);
  const browserCDP = await browser.newBrowserCDPSession();
  await page.route('**/static/visual.js', async route => {
    const response = await route.fetch();
    await route.fulfill({response,body:await response.text()+`;window.activationTimes=[];const measuredActivate=ThoughtsVisual.prototype.activate;ThoughtsVisual.prototype.activate=async function(...args){const start=performance.now();const result=await measuredActivate.apply(this,args);window.activationTimes.push(performance.now()-start);return result;};`});
  });
  await page.goto(url); await expect(page.locator('#status')).toHaveText('Connected');
  const {windowId} = await cdp.send('Browser.getWindowForTarget');
  const fullscreen = async () => { await cdp.send('Browser.setWindowBounds',{windowId,bounds:{windowState:'fullscreen'}}); await page.waitForTimeout(500); };
  await fullscreen();
  const metadata = {revision:process.env.THOUGHTS_PERF_REVISION || execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),logicalCPUs:availableParallelism(),contentionWorkers:Math.min(4,Math.max(1,Math.floor(availableParallelism()/2))),browser:await browser.version(),go:execFileSync('go',['version'],{encoding:'utf8'}).trim(),system:await browserCDP.send('SystemInfo.getInfo'),viewport:await page.evaluate(()=>[innerWidth,innerHeight,devicePixelRatio]),count,appPID};
  const usage = async () => {
    const {processInfo} = await browserCDP.send('SystemInfo.getProcessInfo');
    let browserCPU=0,browserRSS=0,browserPSS=0;
    for (const process of processInfo) {
      browserCPU+=process.cpuTime;
      try { browserRSS+=Number(readFileSync(`/proc/${process.id}/status`,'utf8').match(/VmRSS:\s+(\d+)/)[1]); } catch {}
      try { browserPSS+=Number(readFileSync(`/proc/${process.id}/smaps_rollup`,'utf8').match(/^Pss:\s+(\d+)/m)[1]); } catch {}
    }
    const stat=readFileSync(`/proc/${appPID}/stat`,'utf8').split(') ')[1].split(' ');
    const appCPU=(Number(stat[11])+Number(stat[12]))/ticks;
    const appRSS=Number(readFileSync(`/proc/${appPID}/status`,'utf8').match(/VmRSS:\s+(\d+)/)[1]);
    return {browserCPU,appCPU,browserRSS:browserRSS/1024,browserPSS:browserPSS/1024,appRSS:appRSS/1024};
  };
  const add = row => {rows.push(row);writeFileSync(output,JSON.stringify({metadata,rows},null,2));console.log(JSON.stringify(row));};
  const idle = async image => {
    await page.waitForTimeout(3000);
    for(let run=0;run<count;run++) {
      const a=await usage(),start=performance.now();await page.waitForTimeout(30000);const b=await usage();const seconds=(performance.now()-start)/1000;
      add({phase:'idle',image,run,browserCPUPercent:100*(b.browserCPU-a.browserCPU)/seconds,appCPUPercent:100*(b.appCPU-a.appCPU)/seconds,...b});
    }
  };
  // Measure keydown -> observed terminal row mutation, excluding automation delivery.
  const keyLatency = async key => {
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
  await openVisual(page);await clickControl(page,'Remove background');await expect(screen(page)).not.toContainText('Background darkness:');
  await idle('opaque');
  await page.keyboard.press('Tab');await page.keyboard.press('ArrowRight');await page.keyboard.press('Enter');await expect(screen(page)).toContainText('Background darkness:');await expect(screen(page)).not.toContainText('Loading visual');
  for(const fixture of fixtures) {
    const image=`${fixture.width}x${fixture.height}`;
    for(let run=0;run<count;run++) {
      if(run || fixture!==fixtures[0])await openVisual(page);
      const initial=await usage();let peakBrowserRSS=initial.browserRSS,peakAppRSS=initial.appRSS;
      const timer=setInterval(()=>usage().then(v=>{peakBrowserRSS=Math.max(peakBrowserRSS,v.browserRSS);peakAppRSS=Math.max(peakAppRSS,v.appRSS);}).catch(()=>{}),100);
      try {
      const start=performance.now();await chooseImage(page,{name:'synthetic.png',mimeType:'image/png',buffer:fixture.buffer});
      const previewMs=performance.now()-start;
      if((await screen(page).textContent()).includes('Image fit: Fit'))await clickControl(page,'Image fit: Fit');
      await setDarkness(page,30);
      await page.evaluate(()=>{window.activationTimes=[];});
      const applyStart=performance.now();
        await clickControl(page,'Apply');await expect(screen(page)).not.toContainText('Background darkness:');
        const applyMs=performance.now()-applyStart;
        await page.keyboard.press('ArrowLeft');await expect(page.locator('.xterm-bg-62').filter({hasText:/^Subjects$/})).toBeVisible();await page.keyboard.press('ArrowRight');await expect(page.locator('.xterm-bg-62').filter({hasText:/^Options$/})).toBeVisible();
        const end=await usage();
        add({phase:'import',image,run,bytes:fixture.buffer.length,sha256:createHash('sha256').update(fixture.buffer).digest('hex'),previewMs,applyMs,returnAndSelectionMs:performance.now()-applyStart,activationMs:await page.evaluate(()=>window.activationTimes.at(-1)),stored:await page.locator('#background-image').evaluate(i=>[i.naturalWidth,i.naturalHeight]),browserCPUMs:1000*(end.browserCPU-initial.browserCPU),appCPUMs:1000*(end.appCPU-initial.appCPU),peakBrowserRSS,peakAppRSS,...end});
      } finally {clearInterval(timer);}
    }
    await navigation(image);
    if(fixture===fixtures[0] || fixture===fixtures.at(-1))await idle(image);
    if(fixture===fixtures.at(-1))await navigation(image,true);
  }
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
  await page.screenshot({path:output.replace('.json','-desktop.png')});
  await openVisual(page);await clickControl(page,'Remove background');
} finally {await browser.close();}
