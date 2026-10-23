import {test, expect} from '@playwright/test';

const screen = page => page.locator('.xterm-rows');
async function ready(page) {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Events');
}


async function exposeMountainTerminal(page) {
  await page.addInitScript(() => {
    let TerminalClass;
    Object.defineProperty(window, 'Terminal', {
      configurable: true,
      get: () => TerminalClass,
      set: Base => { TerminalClass = class extends Base {
        constructor(options) { super(options); window.mountainTerminal = this; }
      }; }
    });
  });
}

test('renders newlines at the same column as native terminal output', async ({page}) => {
  await ready(page);
  // A pipe does not apply the PTY's ONLCR output translation. Without the
  // corresponding xterm option, each new line incorrectly moves to the right.
  await expect.poll(() => screen(page).locator('div').nth(2).textContent()).toMatch(/^ Mindset/);
  await expect.poll(() => screen(page).locator('div').nth(4).textContent()).toMatch(/^\s*$/);
  await expect.poll(() => screen(page).locator('div').nth(18).textContent()).toMatch(/^ Events/);
});

test('second tab is busy; quit and reload start independent sessions', async ({page, context}) => {
  await ready(page);
  const second = await context.newPage();
  await second.goto('/');
  await expect(second.locator('#status')).toContainText('Another tab is active');
  await page.keyboard.press('Control+c');
  await expect(screen(page)).toContainText('Exit app?');
  await expect(page.locator('#status')).toHaveText('Connected');
  await page.keyboard.press('y');
  await expect(page.locator('#status')).toContainText('Session ended');
  await second.getByRole('button', {name: 'Reconnect'}).click();
  await expect(second.locator('#status')).toHaveText('Connected');
  await expect(screen(second)).toContainText('Events');
  await second.reload();
  await expect(second.locator('#status')).toHaveText('Connected');
  await expect(screen(second)).toContainText('Events');
});

test('back navigation and default No preserve the session until exit is confirmed', async ({page}) => {
  await ready(page);
  await page.keyboard.press('Tab');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Misc thoughts');
  await expect(screen(page)).not.toContainText('Loading subjects');
  await page.keyboard.press('Control+c');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).not.toContainText('Exit app?');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Subject name:');
  await page.keyboard.type('q');
  await expect(screen(page)).toContainText('Subject name: q');
  await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('Misc thoughts');
  await page.keyboard.press('q');
  await expect(screen(page)).toContainText('Events');
  await page.keyboard.press('q');
  await page.keyboard.press('Control+c');
  await expect(screen(page)).toContainText('Exit app?');
  await expect(screen(page)).toContainText('> No');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Events');
  await expect(page.locator('#status')).toHaveText('Connected');
  await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('Exit app?');
  await page.keyboard.press('Tab');
  await expect(screen(page)).toContainText('> Yes');
  await page.keyboard.press('Enter');
  await expect(page.locator('#status')).toContainText('Session ended');
});

test('reload during filter editing waits for the previous session to finish', async ({page}) => {
  await ready(page);
  await page.keyboard.press('Tab');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Misc thoughts');
  await expect(screen(page)).not.toContainText('Loading');
  await page.keyboard.press('/');
  await page.keyboard.type('Building');
  await expect(screen(page)).toContainText('Filter: Building');
  // The old session must join its already-running cursor command. The new
  // connection may arrive during that short cleanup window.
  await page.reload();
  await expect(screen(page)).toContainText('Events');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).not.toContainText('Filter: Building');
});

test('Ctrl+C copies a terminal selection and requests exit only without a selection', async ({page}) => {
  await ready(page);
  const box = await screen(page).locator('div').first().boundingBox();
  await page.mouse.move(box.x + 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + 100, box.y + box.height / 2, {steps: 10});
  await page.mouse.up();
  await expect(page.locator('.xterm-selection div').first()).toBeVisible();
  await page.keyboard.press('Control+c');
  await expect(page.locator('#status')).toHaveText('Connected');
  await page.mouse.click(box.x + 3, box.y + box.height / 2);
  await page.keyboard.press('Control+c');
  await expect(screen(page)).toContainText('Exit app?');
  await expect(page.locator('#status')).toHaveText('Connected');
  await page.keyboard.press('y');
  await expect(page.locator('#status')).toContainText('Session ended');
});

test('browser paste preserves Unicode and rejects unsupported content without deleting selection', async ({page}) => {
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('console', message => { if (['error','warn'].includes(message.type())) errors.push(message.text()); });
  await ready(page);
  await page.keyboard.press('Tab');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Misc thoughts');
  await expect(screen(page)).not.toContainText('Loading');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Create thought');
  await expect(screen(page)).not.toContainText('Loading');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('Ctrl+S');
  await page.keyboard.insertText('λ');
  await expect(screen(page)).toContainText('λ');
  await page.keyboard.press('Control+g');
  const paste = async text => {
    await page.evaluate(text => navigator.clipboard.writeText(text), text);
    await page.keyboard.press('Control+v');
  };
  await paste('browser draft\n界 café 👩‍💻');
  await expect(screen(page)).toContainText('browser draft');
  await expect(screen(page)).toContainText('界 café 👩‍💻');
  expect((await screen(page).innerText()).match(/browser draft/g)).toHaveLength(1);
  await page.keyboard.press('Control+g');
  await paste('bad\r\nPRIVATE-PASTE-MARKER\t\ufffd');
  await expect(screen(page)).toContainText('Input rejected');
  await expect(screen(page)).toContainText('browser draft');
  await expect(screen(page)).not.toContainText('PRIVATE-PASTE-MARKER');
  await page.setViewportSize({width: 620, height: 700});
  await expect(screen(page)).toContainText('browser draft');
  await expect(screen(page)).toContainText('Ctrl+S');
  await page.keyboard.press('Escape');
  await expect(screen(page)).not.toContainText('Ctrl+S');
  expect(errors).toEqual([]);
});

test('backpressured resize converges to the latest backend size without replaying input', async ({page}) => {
  const clockStart = new Date('2026-01-01T00:00:00Z');
  await page.clock.install({time: clockStart});
  await page.addInitScript(() => {
    window.resizeProbe = {busy: false, frames: [], sockets: []};
    const NativeSocket = window.WebSocket;
    window.WebSocket = class extends NativeSocket {
      constructor(...args) { super(...args); window.resizeProbe.sockets.push(this); }
      get bufferedAmount() { return window.resizeProbe.busy ? 2 * 1024 * 1024 : super.bufferedAmount; }
      send(frame) {
        const kind = String.fromCharCode(frame[0]);
        const size = kind === '2' ? JSON.parse(new TextDecoder().decode(frame.subarray(1))) : null;
        const record = {kind, size, session: window.resizeProbe.sockets.indexOf(this)};
        if (kind === '0' || kind === 'p') record.mutating = kind === 'p' || new TextDecoder().decode(frame.subarray(1)) === '\u001b';
        window.resizeProbe.frames.push(record);
        super.send(frame);
      }
    };
    let TerminalClass;
    Object.defineProperty(window, 'Terminal', {
      configurable: true,
      get: () => TerminalClass,
      set: Base => { TerminalClass = class extends Base {
        constructor(options) { super(options); window.resizeProbe.terminal = this; }
      }; }
    });
  });
  await ready(page);
  // Pause browser timers after initial layout. Advance the retry explicitly,
  // rather than sleeping and hoping a polling timer happened to run.
  await page.clock.pauseAt(new Date(clockStart.getTime() + 60000));
  const geometry = async () => {
    await page.clock.runFor(100);
    return page.evaluate(() => {
      const term = window.resizeProbe.terminal;
      const rows = Array.from({length: term.rows}, (_, i) => term.buffer.active.getLine(i)?.translateToString(true) || '');
      // The entity divider precedes two navigation rows and the final blank
      // row. Measure only its contiguous rows, excluding the Mindset divider;
      // summing wrapped rows still catches stale backend widths.
      const isDivider = row => /^─+$/.test(row);
      const dividerEnd = rows.findLastIndex(isDivider);
      let divider = dividerEnd;
      while (divider > 0 && isDivider(rows[divider - 1])) divider--;
      const dividerWidth = divider < 0 ? 0 : rows.slice(divider, dividerEnd + 1).reduce((sum, row) => sum + row.length, 0);
      return {cols: term.cols, rows: term.rows, dividerWidth, divider};
    });
  };
  const initial = await geometry();
  await expect.poll(geometry).toMatchObject({dividerWidth: initial.cols, divider: initial.rows - 4});
  const currentSession = await page.evaluate(() => window.resizeProbe.sockets.length - 1);
  const before = await page.evaluate(() => window.resizeProbe.frames.length);
  await page.evaluate(() => {
    window.resizeProbe.busy = true;
    window.resizeProbe.terminal.resize(90, 30);
    window.resizeProbe.terminal.resize(100, 35);
  });
  await page.keyboard.press('Escape'); // Would open exit confirmation if replayed.
  await page.evaluate(() => {
    const event = new Event('paste', {bubbles: true, cancelable: true});
    Object.defineProperty(event, 'clipboardData', {value: {getData: () => 'unsent synthetic draft'}});
    document.getElementById('terminal').dispatchEvent(event);
  });
  expect(await page.evaluate(() => window.resizeProbe.frames.length)).toBe(before);
  await page.evaluate(() => { window.resizeProbe.busy = false; });
  await expect.poll(geometry).toMatchObject({cols: 100, rows: 35, dividerWidth: 100, divider: 31});
  const delivered = await page.evaluate(before => window.resizeProbe.frames.slice(before).filter(frame => frame.kind === '2' || frame.mutating), before);
  expect(delivered).toEqual([{kind: '2', size: {cols: 100, rows: 35}, session: currentSession}]);
  await expect(page.locator('#reconnect')).toBeHidden();
  await expect(screen(page)).not.toContainText('unsent synthetic draft');
  await expect(screen(page)).not.toContainText('Exit app?');
  // A pending resize belongs only to its old socket. Reconnect sends the fresh
  // fitted size; it must not send either stale resize or refused input.
  await page.evaluate(currentSession => {
    window.resizeProbe.busy = true;
    window.resizeProbe.terminal.resize(110, 40);
    window.resizeProbe.sockets[currentSession].close();
  }, currentSession);
  await expect(page.locator('#reconnect')).toBeVisible();
  await page.evaluate(() => { window.resizeProbe.busy = false; });
  await page.getByRole('button', {name: 'Reconnect'}).click();
  await expect.poll(geometry).toMatchObject({dividerWidth: initial.cols});
  const newSession = await page.evaluate(() => window.resizeProbe.sockets.length - 1);
  expect(newSession).toBeGreaterThan(currentSession);
  const next = await page.evaluate(newSession => window.resizeProbe.frames.filter(frame => frame.session === newSession && (frame.kind === '2' || frame.mutating)), newSession);
  expect(next).toEqual([{kind: '2', size: {cols: initial.cols, rows: initial.rows}, session: newSession}]);
});

test('day activity focus navigates one day and preserves Events through tuning', async ({page}) => {
  await ready(page);
  await expect(screen(page)).not.toContainText('Days');
  const date = () => screen(page).locator('div').filter({hasText: /^ Events\s+[A-Z]/}).first().textContent();
  const original = await date();
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('1 Total');
  await expect(screen(page)).toContainText('2 Events');
  await page.keyboard.press('ArrowLeft');
  await expect.poll(date).not.toBe(original);
  const previous = await date();
  await page.keyboard.press('2');
  await expect(screen(page)).toContainText('2 Events off');
  const eventRow = () => screen(page).locator('div').evaluateAll(rows => rows.findIndex(row => /^ Events\s+[A-Z]/.test(row.textContent)));
  const defaultRow = await eventRow();
  await page.keyboard.press('d');
  await page.keyboard.press('ArrowRight');
  await expect(screen(page)).toContainText('normal boost');
  for (let step = 0; step < 40; step++) await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('300%');
  await expect.poll(eventRow).toBe(defaultRow);
  expect(await date()).toBe(previous);
  await page.keyboard.press('0');
  await expect(screen(page)).toContainText('low boost');
  await expect.poll(eventRow).toBe(defaultRow);
  await page.keyboard.press('Escape');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('2 Events off');
  expect(await date()).toBe(previous);
  await page.keyboard.press('Escape');
  await expect(screen(page)).not.toContainText('1 Total');
  await expect(screen(page)).not.toContainText('Exit app?');
  await expect(page.locator('#status')).toHaveText('Connected');
});

test('metrics card cycles days without losing its focused view', async ({page}) => {
  await ready(page);
  await page.keyboard.press('End');
  await page.keyboard.press('ArrowDown');
  await expect(screen(page)).toContainText('↑: back/scroll');
  const date = () => screen(page).locator('div').filter({hasText: /^ Events\s+[A-Z]/}).first().textContent();
  const today = await date();
  await page.keyboard.press('ArrowLeft');
  await expect.poll(date).not.toBe(today);
  await expect(screen(page)).toContainText('↑: back/scroll');
  await page.keyboard.press('ArrowRight');
  await expect.poll(date).toBe(today);
  await expect(screen(page)).toContainText('↑: back/scroll');
  await page.keyboard.press('ArrowRight');
  expect(await date()).toBe(today);
  await expect(screen(page)).not.toContainText('Newest first');
});

test('day mountains retain focus colors in a compact window without moving Events', async ({page}) => {
  await exposeMountainTerminal(page);
  await ready(page);
  const mountainHasColor = () => screen(page).locator('div').evaluateAll(rows => {
    const eventRow = rows.findIndex(row => /^ Events\s+[A-Z]/.test(row.textContent));
    if (eventRow < 0) return false;
    return rows.slice(4, eventRow - 1).some(row => [...row.querySelectorAll('span')].some(span => {
      if (!/[\u2801-\u28ff]/.test(span.textContent)) return false;
      const channels = getComputedStyle(span).color.match(/\d+/g)?.map(Number);
      return channels && (channels[0] !== channels[1] || channels[1] !== channels[2]);
    }));
  });
  await expect.poll(mountainHasColor).toBe(false);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await expect.poll(mountainHasColor).toBe(true);
  await page.keyboard.press('ArrowDown');
  await expect.poll(mountainHasColor).toBe(false);
  await page.keyboard.press('Tab'); // Entity picker.
  await expect.poll(mountainHasColor).toBe(true);
  await page.keyboard.press('q'); // Alternate return to Events.
  await expect.poll(mountainHasColor).toBe(false);
  await page.keyboard.press('Tab');
  await expect.poll(mountainHasColor).toBe(true);
  await page.keyboard.press('Tab'); // Mindset.
  await expect.poll(mountainHasColor).toBe(true);
  await page.keyboard.press('Tab'); // Events.
  await expect.poll(mountainHasColor).toBe(false);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  const eventRow = () => screen(page).locator('div').evaluateAll(rows => rows.findIndex(row => /^ Events\s+[A-Z]/.test(row.textContent)));
  await page.setViewportSize({width: 480, height: 480});
  // Wait for the backend's compact layout, not just the browser viewport:
  // the old desktop graph can already satisfy the color assertion.
  let before;
  await expect.poll(async () => {
    before = await page.evaluate(() => {
      const term = window.mountainTerminal;
      const rows = Array.from({length: term.rows}, (_, y) => term.buffer.active.getLine(term.buffer.active.viewportY + y)?.translateToString(true) || '');
      const divider = rows.findLastIndex(row => /^─+$/.test(row));
      const event = rows.findIndex(row => /^ Events\s+[A-Z]/.test(row));
      // Font metrics can give Chromium and Firefox different terminal sizes.
      // Accept only a complete backend frame matching the fitted dimensions.
      return divider === term.rows - 4 && rows[divider].length === term.cols && event > 0 && event < 10 ? event : null;
    });
    return before !== null;
  }).toBe(true);
  await expect.poll(mountainHasColor).toBe(true);
  await page.keyboard.press('d');
  for (let step = 0; step < 40; step++) await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('300%');
  await expect.poll(eventRow).toBe(before);
  await page.keyboard.press('Escape');
  await page.keyboard.press('ArrowDown');
  await expect.poll(mountainHasColor).toBe(false);
});

test('mountain redraws stay above Event cards during tuning and resize', async ({page}) => {
  await exposeMountainTerminal(page);
  await ready(page);
  await page.evaluate(() => {
    const term = window.mountainTerminal;
    window.mountainOverlap = null;
    term.onRender(() => {
      const lines = Array.from({length: term.rows}, (_, y) => term.buffer.active.getLine(term.buffer.active.viewportY + y));
      const heading = lines.findIndex(line => /^ Events\s+[A-Z]/.test(line?.translateToString(true) || ''));
      if (heading < 0 || window.mountainOverlap) return;
      for (let y = heading + 1; y < lines.length; y++) {
        // Synthetic authored text has no Braille. The existing vertical Event
        // and metrics distributions occupy columns 12–21, outside this check.
        for (let x = 24; x < term.cols; x++) {
          if (/[\u2801-\u28ff]/.test(lines[y]?.getCell(x)?.getChars() || '')) {
            window.mountainOverlap = {x, y, heading};
            return;
          }
        }
      }
    });
  });
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('1 Total');
  await page.keyboard.press('d');
  for (let step = 0; step < 40; step++) await page.keyboard.press('ArrowUp');
  await page.keyboard.press('f');
  await page.keyboard.press('f');
  await page.keyboard.press('Escape');
  const eventRow = () => screen(page).locator('div').evaluateAll(rows => rows.findIndex(row => /^ Events\s+[A-Z]/.test(row.textContent)));
  for (const viewport of [{width:480, height:480}, {width:800, height:550}, {width:1200, height:850}]) {
    await page.setViewportSize(viewport);
    for (let step = 0; step < 3; step++) {
      await page.keyboard.press('-');
      await page.keyboard.press('+');
    }
    await expect.poll(eventRow).toBeGreaterThan(0);
  }
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Home');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('←: collapse');
  for (let step = 0; step < 5; step++) await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowLeft');
  await expect(screen(page)).not.toContainText('←: collapse');
  await expect.poll(() => page.evaluate(() => window.mountainOverlap)).toBeNull();
});
