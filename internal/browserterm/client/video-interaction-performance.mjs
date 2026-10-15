// Concrete stress-demo workloads for background-performance.mjs. This measures
// a completed terminal transition, not the first unrelated DOM mutation.
import {expect} from '@playwright/test';
import {readFileSync, writeFileSync} from 'node:fs';
import {createHash} from 'node:crypto';

export async function measureVideoInteractions({page, cdp, url, videoFixturePath, output, add, usage, count, openVisual, chooseImage, clickControl}) {
  const source = readFileSync(videoFixturePath);
  const framing = {fit:'fill', zoom:100, positionX:5000, positionY:5000};
  const origin = new URL(url).origin;
  const imported = await page.request.post(`${url}/visual/video`, {
    headers:{Origin:origin, 'X-Background-Settings':JSON.stringify({darkness:70, framing, speedPercent:100, userPaused:false})}, data:source
  });
  if (!imported.ok()) throw new Error(`Synthetic video import failed: HTTP ${imported.status()}`);
  const prepared = await (await page.request.get(`${url}/visual/background`)).body();
  const poster = await (await page.request.get(`${url}/visual/poster`)).body();
  add({phase:'interaction-fixture', sourceBytes:source.length, sourceSHA256:createHash('sha256').update(source).digest('hex'), preparedSHA256:createHash('sha256').update(prepared).digest('hex')});

  // Fixture substitution is confined to the browser's authorized selected-asset
  // routes. Static mode uses the exact poster, not an unrelated cheaper picture.
  let mode = 'static';
  await page.route('**/visual', async route => {
    const response=await route.fetch(), settings=await response.json();
    await route.fulfill({response,json:{...settings,kind:mode === 'static' ? 'image' : 'video',userPaused:mode === 'paused'}});
  });
  await page.route('**/visual/background*', route => mode === 'static'
    ? route.fulfill({contentType:'image/jpeg', body:poster}) : route.continue());

  async function ready() {
    await page.goto(url);
    await expect(page.locator('#status')).toHaveText('Connected');
    await expect(page.locator('#background-image')).toBeVisible();
    if (mode !== 'static') await expect.poll(() => page.locator('#background-video').evaluate(video => video.paused)).toBe(mode === 'paused');
    await page.waitForFunction(() => window.backgroundProbe && document.querySelector('.xterm-rows').textContent.includes('Make my own timeline feel instant'));
    await page.evaluate(() => {
      const terminal = window.backgroundProbe.terminal;
      window.interactionSnapshot = () => {
        const lines = Array.from({length:terminal.rows}, (_, row) => terminal.buffer.active.getLine(row)?.translateToString(true) || '');
        const text = lines.join('\n');
        let event = '';
        for (let row=0; row<lines.length-1; row++) {
          const column = lines[row].indexOf('╭');
          if (column >= 0 && terminal.buffer.active.getLine(row).getCell(column).getFgColor() === 62) {
            // Activity and fixed timestamps identify this fixture's card. Its
            // live duration changes on End/clock updates, independent of input.
            event=lines[row+1].slice(column+1).split('·').filter((_,index) => index !== 1).join('·').trim();
            break;
          }
        }
        return {text, event, thought:Number(text.match(/│ Thought (\d+)/)?.[1] || 0), day:lines.find(line => /^\s*Events\s+[A-Z]/.test(line)) || '', expanded:text.includes('Newest first')};
      };
    });
  }

  async function key(key, expected) {
    await page.evaluate(expected => {
      const terminal = window.backgroundProbe.terminal;
      window.measuredInteraction = new Promise(resolve => {
        let start;
        const matches = snapshot => {
          if (expected.event) return snapshot.event.includes(expected.event);
          if (expected.eventChanged) return !!snapshot.event && snapshot.event !== expected.eventChanged;
          if (expected.day) return snapshot.day.includes(expected.day) && /\b30 thoughts\b/.test(snapshot.text);
          if (expected.thought) return snapshot.thought === expected.thought;
          if (expected.expanded !== undefined) return snapshot.expanded === expected.expanded && (!expected.expanded || snapshot.thought === 20000);
          return false;
        };
        const finish = value => {clearTimeout(timeout);listener.dispose();document.removeEventListener('keydown', started, true);resolve(value);};
        const started = () => {start=performance.now();};
        document.addEventListener('keydown', started, {once:true,capture:true});
        const listener = terminal.onRender(() => {
          const renderedAt=performance.now();
          if (start === undefined || !matches(window.interactionSnapshot())) return;
          listener.dispose();
          // An animation callback is a rendering opportunity, not proof of GPU
          // presentation. Browser traces supply paint/compositor evidence.
          requestAnimationFrame(() => finish({renderMs:renderedAt-start, arrivalMs:window.lastTerminalArrival-start, opportunityMs:performance.now()-start}));
        });
        const timeout = setTimeout(() => finish({timeout:true, screen:window.interactionSnapshot()}), 3000);
      });
    }, expected);
    await page.keyboard.press(key);
    const measured = await page.evaluate(() => window.measuredInteraction);
    if (measured.timeout) throw new Error(`Transition failed for ${key}: ${JSON.stringify(measured.screen)}`);
    return measured;
  }

  async function workload(name, run, phase, perform) {
    const values = await perform();
    const sorted = field => values.map(value => value[field]).sort((a,b) => a-b);
    const render=sorted('renderMs'), opportunity=sorted('opportunityMs');
    add({phase:'video-interaction', mode, run, window:phase, workload:name, samples:values.length,
      medianMs:render[Math.floor(render.length/2)], p95Ms:render[Math.ceil(render.length*.95)-1],
      opportunityP95Ms:opportunity[Math.ceil(opportunity.length*.95)-1], values});
  }

  async function events(run, phase) {
    const lastEvent = (await page.evaluate(() => window.interactionSnapshot())).event;
    await page.keyboard.press('Home');
    await page.waitForFunction(last => window.interactionSnapshot().event && window.interactionSnapshot().event !== last, lastEvent);
    const firstEvent = (await page.evaluate(() => window.interactionSnapshot())).event;
    // End follows Now without moving selection. Restore the last selection
    // with actual navigation, outside the timed traversal.
    let restored=firstEvent;
    for(let n=0;restored !== lastEvent && n<30;n++) {
      await key('ArrowDown',{eventChanged:restored});
      restored=(await page.evaluate(() => window.interactionSnapshot())).event;
    }
    if(restored !== lastEvent) throw new Error('Could not restore the last Event');
    await workload('event-selection',run,phase,async () => {
      const values=[];
      for(const [direction, endpoint] of [['ArrowUp',firstEvent],['ArrowDown',lastEvent]]) {
        let previous=(await page.evaluate(() => window.interactionSnapshot())).event;
        for(let n=0;previous !== endpoint && n<30;n++) {
          values.push(await key(direction,{eventChanged:previous}));
          previous=(await page.evaluate(() => window.interactionSnapshot())).event;
        }
        if(previous !== endpoint) throw new Error('Did not reach the stress day endpoint');
      }
      return values;
    });
    await workload('event-expansion',run,phase,async () => {
      const values=[];
      for(let n=0;n<10;n++) {values.push(await key('Enter',{expanded:true}));values.push(await key('Escape',{expanded:false}));}
      return values;
    });
    await key('Enter',{expanded:true});
    await workload('expanded-thoughts',run,phase,async () => {
      const values=[];
      // The ongoing stress Event has exactly 200 Thoughts. Traverse the full
      // interval in both directions, including bounded-page fetches.
      for(let n=1;n<200;n++) values.push(await key('ArrowDown',{thought:20000-n}));
      for(let n=198;n>=0;n--) values.push(await key('ArrowUp',{thought:20000-n}));
      return values;
    });
    await key('Escape',{expanded:false});
    const current = await page.evaluate(() => window.interactionSnapshot().day.replace(/^\s*Events\s*/, '').trim());
    const date = new Date(current); date.setDate(date.getDate()-1);
    const previous = date.toLocaleDateString('en-US',{month:'long',day:'numeric',year:'numeric'});
    await workload('day-navigation',run,phase,async () => {
      const values=[];
      for(let n=0;n<10;n++) {values.push(await key('ArrowLeft',{day:previous}));values.push(await key('ArrowRight',{day:current}));}
      return values;
    });
  }

  const runs = process.env.THOUGHTS_PERF_INTERACTION_SMOKE === '1' ? 1 : count;
  for(let run=0;run<runs;run++) for(const selected of ['static','paused','playing'].slice(run%3).concat(['static','paused','playing'].slice(0,run%3))) {
    mode=selected;
    await page.setViewportSize({width:1200,height:850});
    await ready();
    await workload('startup-selection',run,'restore',async () => {
      const values=[];
      for(let n=0;n<5;n++) {values.push(await key('ArrowUp',{event:'Defragment my imaginary desk'}));values.push(await key('ArrowDown',{event:'Make my own timeline feel instant'}));}
      return values;
    });
    if(mode === 'playing') {
      // Exercise the owner's exact Apply -> navigation sequence as well as
      // reload restoration. Native import time is excluded from key latency.
      await openVisual(page);
      await chooseImage(page,{name:`synthetic-interaction-${run}.mp4`,mimeType:'video/mp4',buffer:source});
      await clickControl(page,'Apply');
      await expect(page.locator('.xterm-rows')).not.toContainText('Background darkness:',{timeout:120000});
      await expect(page.locator('#background-video')).toBeVisible();
      await workload('startup-selection',run,'apply',async () => {
        const values=[];
        for(let n=0;n<5;n++) {values.push(await key('ArrowUp',{event:'Defragment my imaginary desk'}));values.push(await key('ArrowDown',{event:'Make my own timeline feel instant'}));}
        return values;
      });
    }
    const idleBefore=await usage(), idleStart=performance.now();
    await page.waitForTimeout(8000); // One complete loop of the reference clip; excluded from input timings.
    const idleAfter=await usage(), idleSeconds=(performance.now()-idleStart)/1000;
    add({phase:'interaction-idle',mode,run,seconds:idleSeconds,browserCPUPercent:100*(idleAfter.browserCPU-idleBefore.browserCPU)/idleSeconds,appCPUPercent:100*(idleAfter.appCPU-idleBefore.appCPU)/idleSeconds,before:idleBefore,...idleAfter});
    const trace = process.env.THOUGHTS_PERF_TRACE === '1' && mode === 'playing' && run === 0;
    if(trace) await cdp.send('Tracing.start',{categories:'devtools.timeline,disabled-by-default-devtools.timeline,cc,media',transferMode:'ReturnAsStream'});
    const before=await usage(), started=performance.now();
    await events(run,'settled');
    await page.keyboard.press('Tab');
    await expect(page.locator('.xterm-rows')).toContainText('Tab/Q: events');
    for(let attempt=0;attempt<3 && !await page.locator('.xterm-bg-62').filter({hasText:/^Thoughts$/}).count();attempt++) {
      const previous=await page.locator('.xterm-bg-62').allTextContents();
      await page.keyboard.press('ArrowLeft');
      await expect.poll(() => page.locator('.xterm-bg-62').allTextContents()).not.toEqual(previous);
    }
    await page.keyboard.press('Enter');
    await expect(page.locator('.xterm-rows')).toContainText('20000 thoughts');
    await key('ArrowDown',{thought:20000}); // All Thoughts begins on its Create row.
    await workload('all-thoughts',run,'settled',async () => {
      const values=[];
      for(let n=1;n<=30;n++) values.push(await key('ArrowDown',{thought:20000-n}));
      for(let n=29;n>=0;n--) values.push(await key('ArrowUp',{thought:20000-n}));
      return values;
    });
    const after=await usage();
    const quality=await page.evaluate(() => {const video=window.backgroundProbe.video;return video ? {...video.getVideoPlaybackQuality().toJSON?.(),frames:video.getVideoPlaybackQuality().totalVideoFrames,dropped:video.getVideoPlaybackQuality().droppedVideoFrames,paused:video.paused,width:video.videoWidth,height:video.videoHeight}:null;});
    add({phase:'interaction-resources',mode,run,seconds:(performance.now()-started)/1000,browserCPUMs:1000*(after.browserCPU-before.browserCPU),appCPUMs:1000*(after.appCPU-before.appCPU),quality,...after});
    await page.screenshot({path:output.replace('.json',`-${mode}-${run}.png`)});
    if(trace) {
      const complete=new Promise(resolve => cdp.once('Tracing.tracingComplete',resolve));
      await cdp.send('Tracing.end');const {stream}=await complete;
      let body='';for(;;) {const part=await cdp.send('IO.read',{handle:stream});body+=part.data;if(part.eof)break;}
      await cdp.send('IO.close',{handle:stream});writeFileSync(output.replace('.json','-trace.json'),body);
    }
    if(run === 0) {
      await page.keyboard.press('Escape');
      await page.setViewportSize({width:650,height:600});
      await ready();await events(run,'narrow');
      await page.screenshot({path:output.replace('.json',`-${mode}-narrow.png`)});
    }
  }
}
