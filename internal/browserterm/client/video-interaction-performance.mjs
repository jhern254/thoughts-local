// Concrete stress-demo workloads for background-performance.mjs. This measures
// a completed terminal transition, not the first unrelated DOM mutation.
import {expect} from '@playwright/test';
import {readFileSync, writeFileSync} from 'node:fs';
import {createHash} from 'node:crypto';

export async function measureVideoInteractions({page, cdp, url, videoFixturePath, output, add, usage, count, qualityStudy, openVisual, chooseImage, clickControl}) {
  const source = readFileSync(videoFixturePath);
  await openVisual(page);
  const previewStart = performance.now();
  await chooseImage(page,{name:'synthetic-interaction.mp4',mimeType:'video/mp4',...(qualityStudy ? {path:videoFixturePath} : {buffer:source})});
  const previewMs = performance.now() - previewStart;
  const applyStart = performance.now();
  await clickControl(page,'Apply');
  await expect(page.locator('.xterm-rows')).not.toContainText('Background darkness:');
  add({phase:'video-import', fixtureInput:qualityStudy ? 'local-file' : 'driver-buffer', previewMs, applyMs:performance.now()-applyStart});
  const prepared = await (await page.request.get(`${url}/visual/background`)).body();
  const poster = await (await page.request.get(`${url}/visual/poster`)).body();
  add({phase:'interaction-fixture', sourceBytes:source.length, sourceSHA256:createHash('sha256').update(source).digest('hex'), preparedSHA256:createHash('sha256').update(prepared).digest('hex')});

  // Fixture substitution is confined to the browser's authorized selected-asset
  // routes. Static mode uses the exact poster, not an unrelated cheaper picture.
  let mode = 'static';
  await page.route('**/visual', async route => {
    const response=await route.fetch(), settings=await response.json();
    await route.fulfill({response,json:{...settings,kind:mode === 'static' ? 'image' : 'video',userPaused:mode === 'paused',...(qualityStudy ? {speedPercent:100} : {})}});
  });
  await page.route('**/visual/background*', route => mode === 'static'
    ? route.fulfill({contentType:'image/jpeg', body:poster}) : route.continue());

  async function ready(reducedMotion = false) {
    await page.goto(url);
    await expect(page.locator('#status')).toHaveText('Connected');
    if (process.env.THOUGHTS_PERF_DISMISS_SPEECH_SETUP === '1') {
      await expect(page.locator('.xterm-rows')).toContainText('Not now');
      await page.locator('.xterm-helper-textarea').focus();
      await page.keyboard.press('Escape');
      await expect(page.locator('.xterm-rows')).not.toContainText('Not now');
    }
    await expect(page.locator('#background-image')).toBeVisible();
    if (mode !== 'static') await expect.poll(() => page.locator('#background-video').evaluate(video => video.paused)).toBe(mode === 'paused' || reducedMotion);
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
    // A fixed preceding-day seed avoids midnight changing the measured workload.
    // Navigate through the real UI; production clocks and timeline rules stay intact.
    if (process.env.THOUGHTS_PERF_PREVIOUS_DAY === '1') {
      const current = await page.evaluate(() => window.interactionSnapshot().day);
      await page.keyboard.press('ArrowLeft');
      await expect.poll(() => page.evaluate(() => window.interactionSnapshot().day)).not.toBe(current);
      await expect(page.locator('.xterm-rows')).not.toContainText('Loading timeline');
      for (let step=0;step<30;step++) {
        const selected = await page.evaluate(() => window.interactionSnapshot().event);
        if (selected.includes('Make my own timeline feel instant')) break;
        await key('ArrowDown',{eventChanged:selected});
      }
      await expect.poll(() => page.evaluate(() => window.interactionSnapshot().event)).toContain('Make my own timeline feel instant');
    }
  }

  async function key(key, expected, repeat = false) {
    const keys = Array.isArray(key) ? key : [key];
    expected = {...expected, keyCount:keys.length};
    await page.evaluate(expected => {
      const terminal = window.backgroundProbe.terminal;
      window.measuredInteraction = new Promise(resolve => {
        let start, lastKey, keyCount = 0;
        const matches = snapshot => {
          if (expected.event) return snapshot.event.includes(expected.event);
          if (expected.eventChanged) return !!snapshot.event && snapshot.event !== expected.eventChanged;
          if (expected.day) return snapshot.day.includes(expected.day) && /\b30 thoughts\b/.test(snapshot.text);
          if (expected.thought) return snapshot.thought === expected.thought;
          if (expected.expanded !== undefined) return snapshot.expanded === expected.expanded && (!expected.expanded || snapshot.thought === 20000);
          if (expected.text) return snapshot.text.includes(expected.text);
          return false;
        };
        const finish = value => {clearTimeout(timeout);listener.dispose();document.removeEventListener('keydown', started, true);resolve(value);};
        const started = () => {lastKey=performance.now();start ??= lastKey;keyCount++;};
        document.addEventListener('keydown', started, {capture:true});
        const listener = terminal.onRender(() => {
          const renderedAt=performance.now(), arrivedAt=window.lastTerminalArrival;
          if (keyCount !== expected.keyCount || !matches(window.interactionSnapshot())) return;
          listener.dispose();
          // An animation callback is a rendering opportunity, not proof of GPU
          // presentation. Browser traces supply paint/compositor evidence.
          requestAnimationFrame(() => finish({renderMs:renderedAt-start, lastKeyMs:renderedAt-lastKey, keys:keyCount, arrivalMs:arrivedAt-start, opportunityMs:performance.now()-start}));
        });
        const timeout = setTimeout(() => finish({timeout:true, screen:window.interactionSnapshot()}), 3000);
      });
    }, expected);
    for (const value of keys) {
      if (repeat) await page.keyboard.down(value);
      else await page.keyboard.press(value);
    }
    if (repeat) await page.keyboard.up(keys.at(-1));
    const measured = await page.evaluate(() => window.measuredInteraction);
    if (measured.timeout) throw new Error(`Transition failed for ${key}: ${JSON.stringify(measured.screen)}`);
    return measured;
  }

  async function workload(name, run, phase, perform) {
    const values = await perform();
    const sorted = field => values.map(value => value[field]).sort((a,b) => a-b);
    const render=sorted('renderMs'), opportunity=sorted('opportunityMs');
    add({phase:'video-interaction', mode, run, window:phase, viewport:page.viewportSize(), workload:name, samples:values.length,
      medianMs:render[Math.floor(render.length/2)], p95Ms:render[Math.ceil(render.length*.95)-1], p99Ms:render[Math.ceil(render.length*.99)-1],
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

  // Focused historical-day probe: every sample requires the requested date
  // AND populated cards. Bursts end on a date not visited earlier in the burst,
  // so an intermediate frame cannot be mistaken for completed navigation.
  if (process.env.THOUGHTS_PERF_DAY_ONLY === '1') {
    const narrow = process.env.THOUGHTS_PERF_INTERACTION_NARROW_ONLY === '1';
    for (let run=0;run<count;run++) for (const selected of ['static','paused','playing'].slice(run%3).concat(['static','paused','playing'].slice(0,run%3))) {
      mode=selected;
      await page.setViewportSize(narrow ? {width:650,height:600} : {width:1200,height:850});
      await ready();
      let date = new Date(await page.evaluate(() => window.interactionSnapshot().day.replace(/^\s*Events\s*/, '').trim()));
      const label = () => date.toLocaleDateString('en-US',{month:'long',day:'numeric',year:'numeric'});
      for (let n=0;n<3;n++) {date.setDate(date.getDate()-1);await key('ArrowLeft',{day:label()});}
      const before=await usage();
      const frames = () => page.evaluate(() => {
        const video=window.backgroundProbe.video;
        if (!video) return null;
        const quality=video.getVideoPlaybackQuality();
        return {frames:quality.totalVideoFrames,dropped:quality.droppedVideoFrames,paused:video.paused,speed:video.playbackRate};
      });
      const framesBefore=await frames(), started=performance.now();
      await workload('historical-day-single',run,narrow?'narrow':'normal',async () => {
        const values=[];
        for (let n=0;n<20;n++) {
          const left=n%2===0;
          date.setDate(date.getDate()+(left?-1:1));
          values.push(await key(left?'ArrowLeft':'ArrowRight',{day:label()}));
        }
        return values;
      });
      const bursts = [
        {name:'same-direction',keys:Array(8).fill('ArrowLeft'),delta:-8},
        {name:'repeat-keydown',keys:Array(8).fill('ArrowRight'),delta:8,repeat:true},
        {name:'alternating',keys:[...Array.from({length:10},(_,i)=>i%2?'ArrowRight':'ArrowLeft'),'ArrowLeft','ArrowLeft','ArrowLeft'],delta:-3},
      ];
      for (const burst of bursts) {
        date.setDate(date.getDate()+burst.delta);
        const value=await key(burst.keys,{day:label()},burst.repeat);
        // Allow obsolete replies a chance to arrive, then verify the accepted
        // day remains the accumulated destination and the panel stays focused.
        await page.waitForTimeout(200);
        const snapshot=await page.evaluate(() => window.interactionSnapshot());
        expect(snapshot.day).toContain(label());
        expect(snapshot.event).not.toBe('');
        add({phase:'historical-day-burst',mode,run,burst:burst.name,expectedDay:label(),actualDay:snapshot.day,...value});
      }
      const seconds=(performance.now()-started)/1000, after=await usage();
      add({phase:'historical-day-resources',mode,run,seconds,browserCPUMs:1000*(after.browserCPU-before.browserCPU),appCPUMs:1000*(after.appCPU-before.appCPU),browserRSS:after.browserRSS,appRSS:after.appRSS,framesBefore,framesAfter:await frames()});
      if (run===0) await page.screenshot({path:output.replace(/\.json$/,`-${mode}.png`)});
    }
    return;
  }

  const conditionsOnly = qualityStudy && process.env.THOUGHTS_PERF_CONDITIONS_ONLY === '1';
  const runs = conditionsOnly ? 0 : process.env.THOUGHTS_PERF_INTERACTION_SMOKE === '1' ? 1 : count;
  const narrowOnly = process.env.THOUGHTS_PERF_INTERACTION_NARROW_ONLY === '1';
  for(let run=0;run<runs;run++) for(const selected of narrowOnly ? ['playing'] : ['static','paused','playing'].slice(run%3).concat(['static','paused','playing'].slice(0,run%3))) {
    mode=selected;
    await page.setViewportSize(narrowOnly ? {width:650,height:600} : {width:1200,height:850});
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
      // Quality qualification measures repeated interaction with the same exact
      // prepared bytes. Its one-time import is recorded separately above.
      if (!qualityStudy) await chooseImage(page,{name:`synthetic-interaction-${run}.mp4`,mimeType:'video/mp4',buffer:source});
      await clickControl(page,'Apply');
      await expect(page.locator('.xterm-rows')).not.toContainText('Background darkness:',{timeout:120000});
      await expect(page.locator('#background-video')).toBeVisible();
      await workload('startup-selection',run,qualityStudy ? 'apply-settings' : 'apply',async () => {
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
    await events(run,narrowOnly ? 'narrow-repeat' : 'settled');
    await page.keyboard.press('Tab');
    await expect(page.locator('.xterm-rows')).toContainText('Tab: mindset • Q: events');
    for(let attempt=0;attempt<3 && !await page.locator('.xterm-bg-62').filter({hasText:/^Thoughts$/}).count();attempt++) {
      const previous=await page.locator('.xterm-bg-62').allTextContents();
      await page.keyboard.press('ArrowLeft');
      await expect.poll(() => page.locator('.xterm-bg-62').allTextContents()).not.toEqual(previous);
    }
    await page.keyboard.press('Enter');
    await expect(page.locator('.xterm-rows')).toContainText('20000 thoughts');
    await key('ArrowDown',{thought:20000}); // All Thoughts begins on its Create row.
    await workload('all-thoughts',run,narrowOnly ? 'narrow-repeat' : 'settled',async () => {
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
    if(run === 0 && !narrowOnly) {
      await page.keyboard.press('Escape');
      await page.setViewportSize({width:650,height:600});
      await ready();await events(run,'narrow');
      await page.screenshot({path:output.replace('.json',`-${mode}-narrow.png`)});
    }
  }
  if (qualityStudy) {
    mode = 'playing';
    await page.setViewportSize({width:1200,height:850});
    await ready();
    const playback = () => page.evaluate(() => {
      const video = document.querySelector('#background-video');
      const quality = video.getVideoPlaybackQuality();
      return {frames:quality.totalVideoFrames,dropped:quality.droppedVideoFrames,paused:video.paused,speed:video.playbackRate,terminalConnections:window.perfTerminalConnections};
    });
    const sample = async condition => {
      for (let run=0;run<count;run++) {
        const before=await usage(), qualityBefore=await playback(), start=performance.now();
        await page.waitForTimeout(8000);
        const after=await usage(), seconds=(performance.now()-start)/1000, quality=await playback();
        add({phase:'quality-condition',condition,run,seconds,browserCPUPercent:100*(after.browserCPU-before.browserCPU)/seconds,appCPUPercent:100*(after.appCPU-before.appCPU)/seconds,before,qualityBefore,quality,...after});
      }
    };
    await sample('normal-100');
    await page.keyboard.press('t');
    await expect(page.locator('.xterm-rows')).toContainText('Create thought');
    let typed='qx-';
    await page.keyboard.type(typed);
    await expect(page.locator('.xterm-rows')).toContainText(typed);
    for (let run=0;run<count;run++) {
      await workload('typing',run,'normal-100',async () => {
        const values=[];
        for (const character of 'cpuheadroom') {
          typed+=character;
          values.push(await key(character,{text:typed}));
        }
        return values;
      });
    }
    await page.keyboard.press('F8');
    await expect.poll(() => page.evaluate(() => window.voiceProbe.requests)).toBe(1);
    await page.evaluate(() => window.voiceProbe.grant(0));
    await expect(page.locator('#status')).toHaveText('Transcribing…');
    await sample('synthetic-microphone');
    await page.keyboard.press('F8');
    await expect(page.locator('.xterm-rows')).toContainText('F8: Record');
    await page.keyboard.press('Escape');
    await openVisual(page);
    await clickControl(page,'Playback speed: 100%');
    await page.keyboard.press('Home');
    for (let step=0;step<5;step++) await page.keyboard.press('ArrowRight');
    await expect(page.locator('.xterm-rows')).toContainText('Playback speed: 50%');
    await clickControl(page,'Apply');
    await expect(page.locator('.xterm-rows')).not.toContainText('Background darkness:');
    await sample('slow-50');
    await openVisual(page);
    await clickControl(page,'Playback speed: 50%');
    await page.keyboard.press('End');
    await expect(page.locator('.xterm-rows')).toContainText('Playback speed: 100%');
    await clickControl(page,'Pause video');
    await clickControl(page,'Apply');
    await expect(page.locator('.xterm-rows')).not.toContainText('Background darkness:');
    await sample('paused');
    await openVisual(page);
    await clickControl(page,'Play video');
    await clickControl(page,'Apply');
    await expect(page.locator('.xterm-rows')).not.toContainText('Background darkness:');
    await page.emulateMedia({reducedMotion:'reduce'});
    // Deliberate Play overrides reduced motion for one session. A fresh page
    // measures the default reduced-motion behavior without that override.
    await ready(true);
    await expect.poll(() => playback().then(value=>value.paused)).toBe(true);
    await sample('reduced-motion');
    await page.emulateMedia({reducedMotion:'no-preference'});
  }
}
