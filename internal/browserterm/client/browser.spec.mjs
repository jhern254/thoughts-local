import {readFileSync} from 'node:fs';
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
  await expect.poll(() => screen(page).locator('div').nth(3).textContent()).toMatch(/^\s*$/);
  await expect.poll(() => screen(page).locator('div').nth(9).textContent()).toMatch(/^ Events/);
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
  await page.keyboard.press('Control+q');
  await expect(screen(page)).toContainText('Discard draft?');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Enter');
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
  const date = async () => (await screen(page).locator('div').filter({hasText: /^ Events\s+[A-Z]/}).first().textContent()).match(/^ Events\s+[A-Z][a-z]+ \d+, \d{4}/)[0];
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
  await expect(screen(page)).toContainText('normal boost');
  await expect(screen(page)).toContainText('100%');
  await page.keyboard.press('ArrowRight');
  await expect(screen(page)).toContainText('medium boost');
  for (let step = 0; step < 40; step++) await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('300%');
  await expect.poll(eventRow).toBe(defaultRow);
  expect(await date()).toBe(previous);
  await page.keyboard.press('0');
  await expect(screen(page)).toContainText('normal boost');
  await expect(screen(page)).toContainText('100%');
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

test('mountain status stays beside the Events date or in the narrow footer above a shared baseline', async ({page}) => {
  await exposeMountainTerminal(page);
  await ready(page);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('d');
  const snapshot = () => page.evaluate(() => {
    const term = window.mountainTerminal;
    const rows = Array.from({length: term.rows}, (_, y) => term.buffer.active.getLine(term.buffer.active.viewportY + y)?.translateToString(true) || '');
    const eventRow = rows.findIndex(row => /^ Events\s+[A-Z]/.test(row));
    const markerLine = term.buffer.active.getLine(term.buffer.active.viewportY + eventRow - 1);
    const markerColumns = Array.from({length: term.cols}, (_, x) => x).filter(x => markerLine?.getCell(x)?.getBgColor() === 62);
    return {rows, cols: term.cols, eventRow, markerColumns};
  });
  await expect.poll(async () => (await snapshot()).rows[9]).toMatch(/normal boost · 100% · filled$/);
  let frame = await snapshot();
  expect(frame.rows[9].length).toBe(frame.cols);
  expect(frame.rows.slice(4, 8).join('')).not.toMatch(/boost|Loading/);
  expect([...frame.rows[7]].every(glyph => (glyph.codePointAt(0) - 0x2800 & 0xc0) === 0xc0)).toBe(true);
  expect(frame.rows[8].trim()).toBe('');
  expect(frame.markerColumns).toHaveLength(1);
  const firstMarker = frame.markerColumns[0];
  await page.keyboard.press('Escape');
  await page.keyboard.press('ArrowLeft');
  await expect.poll(async () => (await snapshot()).markerColumns[0]).toBeLessThan(firstMarker);
  await page.keyboard.press('ArrowRight');
  await expect.poll(async () => (await snapshot()).markerColumns[0]).toBe(firstMarker);
  await page.keyboard.press('d');
  await expect(screen(page)).toContainText('normal boost · 100% · filled');
  await page.setViewportSize({width: 380, height: 480});
  await expect.poll(async () => {
    frame = await snapshot();
    const footer = frame.rows.findLastIndex(row => /^─+$/.test(row));
    return frame.cols < 50 && footer === frame.rows.length - 4 && frame.rows.slice(footer + 1).some(row => row.includes('normal boost · 100% · filled'));
  }).toBe(true);
  // Normal zoom has at most four mountain rows plus a separate marker.
  // Short terminals return plot rows to the timeline before hiding cards.
  const expectedRow = 5 + Math.max(1, Math.min(4, frame.rows.length - 23));
  expect(frame.eventRow).toBe(expectedRow);
  expect(frame.rows[frame.eventRow]).not.toContain('boost');
  expect(frame.rows[frame.eventRow - 1].trim()).toBe('');
  expect(frame.markerColumns).toHaveLength(1);
  expect([...frame.rows[frame.eventRow - 2]].every(glyph => (glyph.codePointAt(0) - 0x2800 & 0xc0) === 0xc0)).toBe(true);
  expect(frame.rows.every(row => row.length <= frame.cols)).toBe(true);
});

test('activity loading clears after rapid day panning without reconnecting', async ({page}) => {
  await exposeMountainTerminal(page);
  let connections = 0;
  page.on('websocket', () => connections++);
  await ready(page);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('1 Total');
  const date = async () => (await screen(page).locator('div').filter({hasText: /^ Events\s+[A-Z]/}).first().textContent()).match(/^ Events\s+([A-Z][a-z]+ \d+, \d{4})/)[1];
  const original = await date();
  const previous = new Date(original);
  previous.setDate(previous.getDate() - 40);
  const expected = previous.toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'});
  // Cross the month viewport repeatedly, rather than only moving its marker.
  for (let step = 0; step < 40; step++) await page.keyboard.press('ArrowLeft');
  await expect.poll(date).toBe(expected);
  await expect(screen(page)).not.toContainText('Loading activity');
  for (let step = 0; step < 40; step++) await page.keyboard.press('ArrowRight');
  await expect.poll(date).toBe(original);
  await expect(screen(page)).not.toContainText('Loading activity');
  await page.keyboard.press('ArrowDown');
  await expect(screen(page)).not.toContainText('1 Total');
  await expect(screen(page)).not.toContainText('Loading activity');
  expect(connections).toBe(1);
});

test('day block stays on the border while the viewport pans', async ({page}) => {
  await exposeMountainTerminal(page);
  await ready(page);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('1 Total');
  await page.evaluate(() => {
    const term = window.mountainTerminal;
    window.edgeWatch = {direction: 'left', reached: false, violations: []};
    window.edgeCapture = term.onRender(() => {
      const rows = Array.from({length: term.rows}, (_, y) => term.buffer.active.getLine(term.buffer.active.viewportY + y));
      const event = rows.findIndex(row => /^ Events\s/.test(row?.translateToString(true) || ''));
      if (event < 1) return;
      const columns = Array.from({length: term.cols}, (_, x) => x).filter(x => rows[event - 1]?.getCell(x)?.getBgColor() === 62);
      if (columns.length !== 1) return;
      const target = window.edgeWatch.direction === 'left' ? 0 : term.cols - 1;
      if (columns[0] === target) window.edgeWatch.reached = true;
      else if (window.edgeWatch.reached) window.edgeWatch.violations.push({direction: window.edgeWatch.direction, column: columns[0], target});
    });
  });
  const date = async () => (await screen(page).locator('div').filter({hasText: /^ Events\s+[A-Z]/}).first().textContent()).match(/^ Events\s+([A-Z][a-z]+ \d+, \d{4})/)[1];
  const original = await date();
  const previous = new Date(original);
  previous.setDate(previous.getDate() - 45);
  for (let step = 0; step < 45; step++) await page.keyboard.press('ArrowLeft');
  await expect.poll(date).toBe(previous.toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'}));
  await expect.poll(() => page.evaluate(() => window.edgeWatch.reached)).toBe(true);
  expect(await page.evaluate(() => window.edgeWatch.violations)).toEqual([]);
  await page.evaluate(() => { window.edgeWatch.direction = 'right'; window.edgeWatch.reached = false; });
  for (let step = 0; step < 45; step++) await page.keyboard.press('ArrowRight');
  await expect.poll(date).toBe(original);
  await expect.poll(() => page.evaluate(() => window.edgeWatch.reached)).toBe(true);
  expect(await page.evaluate(() => window.edgeWatch.violations)).toEqual([]);
  await page.evaluate(() => window.edgeCapture.dispose());
});

test('Now view cycles days while retaining its latest-card and metrics layout', async ({page}) => {
  await ready(page);
  await page.keyboard.press('End');
  await expect(screen(page)).toContainText('↑: latest card');
  const date = () => screen(page).locator('div').filter({hasText: /^ Events\s+[A-Z]/}).first().textContent();
  const cards = () => screen(page).locator('div').evaluateAll(rows => rows.filter(row => row.textContent.includes('╭')).length);
  const original = await date();
  await expect.poll(cards).toBe(2);
  // The relative seed spans three hours and can cross local midnight.
  // Two dates back is empty regardless of the time this test is run.
  await page.keyboard.press('ArrowLeft');
  await expect.poll(date).not.toBe(original);
  const previous = await date();
  await page.keyboard.press('ArrowLeft');
  await expect.poll(date).not.toBe(previous);
  await expect(screen(page)).toContainText('↑: latest card');
  await expect(screen(page)).toContainText('Subject time');
  await expect.poll(cards).toBe(1); // No invented Event on the guaranteed-empty day.
  await page.keyboard.press('ArrowRight');
  await expect.poll(date).toBe(previous);
  await page.keyboard.press('ArrowRight');
  await expect.poll(date).toBe(original);
  await expect(screen(page)).toContainText('↑: latest card');
  await expect.poll(cards).toBe(2);
  await page.keyboard.press('ArrowRight');
  await expect(screen(page)).not.toContainText('←: collapse');
  await expect.poll(date).toBe(original);
});

test('Mindset arrows switch days while Enter remains its Thought-opening key', async ({page}) => {
  await ready(page);
  await page.keyboard.press('Shift+Tab');
  await expect(screen(page)).toContainText('←/→: day');
  await expect(screen(page)).toContainText('Enter: open');
  const date = () => screen(page).locator('div').filter({hasText: /^ Events\s+[A-Z]/}).first().textContent();
  const original = await date();
  await page.keyboard.press('ArrowLeft');
  await expect.poll(date).not.toBe(original);
  await expect(screen(page)).toContainText('↑/↓: read/select');
  await page.keyboard.press('ArrowRight');
  await expect.poll(date).toBe(original);
  await expect(screen(page)).toContainText('↑/↓: read/select');
  await page.keyboard.press('ArrowRight');
  await expect.poll(date).toBe(original);
  await expect(screen(page)).toContainText('Enter: open');
});

test('metrics card cycles days without losing its focused view', async ({page}) => {
  await ready(page);
  await page.keyboard.press('End');
  await page.keyboard.press('ArrowDown');
  await expect(screen(page)).toContainText('↑: back/scroll');
  const date = async () => (await screen(page).locator('div').filter({hasText: /^ Events\s+[A-Z]/}).first().textContent()).match(/^ Events\s+[A-Z][a-z]+ \d+, \d{4}/)[0];
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

test('day mountains preserve focus palettes in a compact window without moving Events', async ({page}) => {
  await exposeMountainTerminal(page);
  await ready(page);
  const mountainHasColor = () => screen(page).locator('div').evaluateAll(rows => {
    const eventRow = rows.findIndex(row => /^ Events\s+[A-Z]/.test(row.textContent));
    if (eventRow < 0) return false;
    return rows.slice(4, eventRow).some(row => [...row.querySelectorAll('span')].some(span => {
      if (!/[\u2801-\u28ff]/.test(span.textContent)) return false;
      // Count the actual teal/amber components, not the purple date marker.
      return ['rgb(0, 175, 175)', 'rgb(215, 175, 95)'].includes(getComputedStyle(span).color);
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
      // A coherent desktop frame can still be present before ResizeObserver
      // delivers the quarter-window size. Require that width before sampling.
      return term.cols < 60 && divider === term.rows - 4 && rows[divider].length === term.cols && event > 0 && event < 10 ? event : null;
    });
    return before !== null;
  }).toBe(true);
  // Font metrics can leave one plot row here, where enabled Total stays grey.
  // Multiple rows retain the highlighted component palette.
  await expect.poll(mountainHasColor).toBe(before > 6);
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
            window.mountainOverlap = {
              x, y, heading, cols: term.cols, rows: term.rows,
              phase: window.mountainPhase,
              synchronized: term.modes.synchronizedOutputMode,
              buffer: lines[y]?.translateToString(true),
              painted: document.querySelectorAll('.xterm-rows > div')[y]?.textContent,
            };
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
  // One-day zoom and returning retain the same compact Events boundary.
  for (let step = 0; step < 16; step++) await page.keyboard.press('+');
  await expect.poll(eventRow).toBe(9);
  await page.screenshot({path:'/tmp/pr92-mountains/app-maximum-'+test.info().project.name+'.png'});
  await page.keyboard.press('-');
  await expect.poll(eventRow).toBe(9);
  for (const viewport of [{width:480, height:480}, {width:800, height:550}, {width:1200, height:850}]) {
    await page.evaluate(size => { window.mountainPhase = `resize ${size.width}x${size.height}`; }, viewport);
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

test('day graph keeps the same normal profile in medium and fullscreen windows', async ({page}) => {
  await exposeMountainTerminal(page);
  await ready(page);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('1 Total');
  await page.keyboard.press('d');
  await expect(screen(page)).toContainText('normal boost');
  await expect(screen(page)).toContainText('100%');
  await page.keyboard.press('Escape');
  await expect(screen(page)).not.toContainText('normal boost');
  await expect(screen(page)).not.toContainText('Loading activity');
  const plot = () => page.evaluate(() => {
    const term = window.mountainTerminal;
    return [4, 5, 6, 7].map(y => term.buffer.active.getLine(term.buffer.active.viewportY + y)?.translateToString() || '');
  });
  const baseline = await plot();
  for (const height of [700, 1080, 850]) {
    await page.setViewportSize({width: 1200, height});
    let frame;
    await expect.poll(async () => {
      frame = await page.evaluate(() => {
        const term = window.mountainTerminal;
        const rows = Array.from({length: term.rows}, (_, y) => term.buffer.active.getLine(term.buffer.active.viewportY + y)?.translateToString() || '');
        const footer = rows.findLastIndex(row => /^─+$/.test(row.trimEnd()));
        return {
          ready: footer === term.rows - 4 && rows[footer].trimEnd().length === term.cols && /^ Events\s/.test(rows[9]),
          plot: rows.slice(4, 8),
        };
      });
      return frame.ready;
    }).toBe(true);
    // Keep readiness and geometry in one snapshot: a resize can repaint again
    // between separate evaluations. An incorrect coherent profile still fails.
    expect(frame.plot).toEqual(baseline);
  }
});

test('Events focus supports zoom and month reset without consuming form or style keys', async ({page}) => {
  await exposeMountainTerminal(page);
  let connections = 0;
  page.on('websocket', () => connections++);
  await ready(page);
  await expect(screen(page)).not.toContainText('Loading activity');
  await expect(screen(page)).toContainText('0: reset zoom');
  const plot = () => page.evaluate(() => {
    const term = window.mountainTerminal;
    return [4, 5, 6, 7].map(y => term.buffer.active.getLine(term.buffer.active.viewportY + y)?.translateToString() || '');
  });
  const date = async () => (await screen(page).locator('div').filter({hasText: /^ Events\s+[A-Z]/}).first().textContent()).match(/^ Events\s+[A-Z][a-z]+ \d+, \d{4}/)[0];
  const originalDate = await date();
  const original = await plot();
  await page.keyboard.press('+');
  await expect.poll(plot).not.toEqual(original);
  await expect(screen(page)).not.toContainText('1 Total');
  expect(await date()).toBe(originalDate);
  await page.keyboard.press('-');
  await expect(screen(page)).not.toContainText('Loading activity');
  await page.keyboard.press('0');
  await expect.poll(plot).toEqual(original);
  expect(await date()).toBe(originalDate);
  await expect(screen(page)).not.toContainText('1 Total');
  await page.keyboard.press('Home');
  await page.keyboard.press('Enter');
  await expect(screen(page)).toContainText('←: collapse');
  await page.keyboard.press('+');
  await expect.poll(plot).not.toEqual(original);
  await page.keyboard.press('0');
  await expect.poll(plot).toEqual(original);
  await expect(screen(page)).toContainText('←: collapse');
  await page.keyboard.press('Escape');
  await page.keyboard.press('d');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('105%');
  await page.keyboard.press('0');
  await expect(screen(page)).toContainText('normal boost');
  await expect(screen(page)).toContainText('100%');
  await page.keyboard.press('Escape');
  await page.keyboard.press('n');
  await page.keyboard.type('0+-');
  await expect(screen(page)).toContainText('0+-');
  await page.keyboard.press('Escape');
  await expect(screen(page)).toContainText('Events');
  expect(connections).toBe(1);
});


test('quarter window retains its previous compact mountain height through zoom and resizing', async ({page},testInfo) => {
  await exposeMountainTerminal(page);
  await ready(page);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('1 Total');
  for (const viewport of [{width:1200,height:850},{width:480,height:480},{width:1200,height:850}]) {
    await page.setViewportSize(viewport);
    const snapshot = () => page.evaluate(() => {
      const term=window.mountainTerminal;
      const rows=Array.from({length:term.rows},(_,y)=>term.buffer.active.getLine(term.buffer.active.viewportY+y)?.translateToString(true)||'');
      const footer=rows.findLastIndex(row=>/^─+$/.test(row));
      return {cols:term.cols,rows:term.rows,eventRow:rows.findIndex(row=>/^ Events\s+[A-Z]/.test(row)),ready:footer===term.rows-4&&rows[footer]?.length===term.cols};
    });
    let baseline;
    await expect.poll(async()=>{
      baseline=await snapshot();
      // The quarter window keeps its existing shorter band; it must neither
      // expand on zoom nor lose more rows after an async resize completes.
      return baseline.ready && (viewport.width===480
        ? baseline.cols<60 && baseline.eventRow>=6 && baseline.eventRow<=9
        : baseline.cols>100 && baseline.eventRow===9);
    }).toBe(true);
    for(let step=0;step<16;step++)await page.keyboard.press('+');
    await page.keyboard.press('d');
    await expect(screen(page)).toContainText('normal boost');
    expect((await snapshot()).eventRow).toBe(baseline.eventRow);
    if(viewport.width===480)await page.screenshot({path:`/tmp/pr92-mountains/compact-quarter-${testInfo.project.name}.png`});
    await page.keyboard.press('Escape');
    await page.keyboard.press('0');
    expect((await snapshot()).eventRow).toBe(baseline.eventRow);
  }
});

test('one-row mountains show grey Total and restore layers when the window grows', async ({page},testInfo) => {
  await exposeMountainTerminal(page);
  await ready(page);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('1 Total');
  const snapshot = () => page.evaluate(() => {
    const term = window.mountainTerminal;
    const lines = Array.from({length:term.rows},(_,y)=>term.buffer.active.getLine(term.buffer.active.viewportY+y));
    const text = lines.map(line=>line?.translateToString(true)||'');
    const eventRow = text.findIndex(row=>/^ Events\s+[A-Z]/.test(row));
    const footer = text.findLastIndex(row=>/^─+$/.test(row));
    const series = new Set();
    for(let y=4;y<eventRow-1;y++)for(let x=0;x<term.cols;x++){
      const cell=lines[y]?.getCell(x);
      if(/[\u2801-\u28ff]/.test(cell?.getChars()||''))series.add(cell.getFgColor());
    }
    return {cols:term.cols,rows:term.rows,eventRow,series:[...series].sort((a,b)=>a-b),complete:footer===term.rows-4&&text[footer]?.length===term.cols};
  });
  for(const mode of ['filled','outline']){
    if(mode==='outline'){
      await page.keyboard.press('d');await page.keyboard.press('f');
      await expect(screen(page)).toContainText('outline');await page.keyboard.press('Escape');
    }
    await page.setViewportSize({width:960,height:450});
    await expect.poll(async()=>{const state=await snapshot();return state.complete&&state.cols<120&&state.rows<=24&&state.eventRow===6;}).toBe(true);
    await expect.poll(async()=>(await snapshot()).series).toEqual([240]);
    await page.screenshot({path:testInfo.outputPath('one-row-total-'+mode+'.png')});
    await page.keyboard.press('1');
    await expect(screen(page)).toContainText('1 Total off');
    await expect.poll(async()=>(await snapshot()).series.some(color=>color===37||color===179)).toBe(true);
    await page.keyboard.press('1');
    await expect(screen(page)).not.toContainText('1 Total off');
    await expect.poll(async()=>(await snapshot()).series).toEqual([240]);
    await page.setViewportSize({width:1200,height:850});
    await expect.poll(async()=>{const state=await snapshot();return state.complete&&state.cols>120&&state.eventRow===9;}).toBe(true);
    await expect.poll(async()=>(await snapshot()).series).toEqual([37,179,240]);
  }
});

test('day marker remains visible at today in a small window', async ({page},testInfo) => {
  await exposeMountainTerminal(page);
  await ready(page);
  await page.keyboard.press('Home');await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('1 Total');
  const snapshot=()=>page.evaluate(()=>{
    const term=window.mountainTerminal;
    const lines=Array.from({length:term.rows},(_,y)=>term.buffer.active.getLine(term.buffer.active.viewportY+y));
    const text=lines.map(line=>line?.translateToString(true)||'');
    const event=text.findIndex(row=>/^ Events\s+[A-Z]/.test(row));
    const footer=text.findLastIndex(row=>/^─+$/.test(row));
    const columns=Array.from({length:term.cols},(_,x)=>x).filter(x=>lines[event-1]?.getCell(x)?.getBgColor()===62);
    return {cols:term.cols,rows:term.rows,event,date:text[event],columns,complete:footer===term.rows-4&&text[footer]?.length===term.cols};
  });
  for(const viewport of [{width:960,height:450},{width:480,height:480},{width:960,height:400}]){
    await page.setViewportSize(viewport);
    await expect.poll(async()=>{const state=await snapshot();const expected=5+Math.max(1,Math.min(4,state.rows-23));return state.complete&&state.cols<(viewport.width===480?60:120)&&state.rows<30&&state.event===expected;}).toBe(true);
    const initial=await snapshot();
    await expect.poll(async()=>{const state=await snapshot();return state.columns.map(column=>column-(state.cols-1));}).toEqual([0]);
    for(let round=0;round<3;round++){
      for(let step=0;step<45;step++)await page.keyboard.press('ArrowLeft');
      await expect.poll(async()=>(await snapshot()).columns).toEqual([0]);
      for(let step=0;step<45;step++)await page.keyboard.press('ArrowRight');
      await expect.poll(async()=>(await snapshot()).date).toBe(initial.date);
      await expect.poll(async()=>{const state=await snapshot();return state.columns.map(column=>column-(state.cols-1));}).toEqual([0]);
    }
    await page.screenshot({path:testInfo.outputPath('today-marker-'+viewport.width+'x'+viewport.height+'.png')});
  }
});

test('terminal character edits honor the visible right margin after shrinking', async ({page}) => {
  await ready(page);
  // Load the pinned, unpatched package only in this test page to prove the
  // original failure, not just the corrected output. CSP still uses self scripts.
  await page.evaluate(()=>{window.PatchedTerminal=window.Terminal;});
  await page.route('**/unpatched-xterm-probe.js',route=>route.fulfill({contentType:'text/javascript',body:readFileSync(new URL('node_modules/@xterm/xterm/lib/xterm.js',import.meta.url),'utf8')}));
  await page.addScriptTag({url:'/unpatched-xterm-probe.js'});
  const original=await page.evaluate(async()=>{
    const host=document.createElement('div');document.body.append(host);
    const term=new Terminal({cols:120,rows:10,allowProposedApi:true});term.open(host);
    const write=text=>new Promise(resolve=>term.write(text,resolve));
    try{
      await write('\x1b[?1049h');term.resize(111,10);
      await write('\x1b[6;107H\x1b[48;5;62m\x1b[P\x1b[m');
      const line=term.buffer.active.getLine(5);
      return {width:line.length,blue:Array.from({length:line.length},(_,x)=>x).filter(x=>line.getCell(x).getBgColor()===62)};
    }finally{term.dispose();host.remove();window.Terminal=window.PatchedTerminal;}
  });
  expect(original).toEqual({width:120,blue:[119]});
  const cases=await page.evaluate(async()=>{
    const results=[];
    // CSI P deletes characters; CSI @ inserts them. A shrunken alternate
    // screen can retain longer lines, but both edits must stop at the viewport.
    for(const operation of ['P','@'])for(const count of [1,4]){
      const host=document.createElement('div');host.style.cssText='position:fixed;left:-10000px;width:1200px;height:200px';document.body.append(host);
      const term=new Terminal({cols:120,rows:10,allowProposedApi:true});term.open(host);
      const write=text=>new Promise(resolve=>term.write(text,resolve));
      try{
        await write('\x1b[?1049h');term.resize(111,10);
        const resizedWidth=term.buffer.active.getLine(5).length;
        await write('\x1b[6;5H\x1b[38;5;179m界a\x1b[m');
        if(operation==='@')await write('\x1b[6;111H\x1b[48;5;62m \x1b[m');
        await write('\x1b[6;107H'+(operation==='P'?'\x1b[48;5;62m':'')+'\x1b['+count+operation+'\x1b[m');
        const inspect=()=>{
          const line=term.buffer.active.getLine(5);
          return {blue:Array.from({length:line.length},(_,x)=>x).filter(x=>line.getCell(x).getBgColor()===62),unicode:line.getCell(4).getChars(),foreground:line.getCell(4).getFgColor(),following:line.getCell(6).getChars()};
        };
        const small=inspect();term.resize(120,10);
        results.push({operation,count,resizedWidth,small,expanded:inspect()});
      }finally{term.dispose();host.remove();}
    }
    return results;
  });
  for(const {operation,count,resizedWidth,small,expanded} of cases){
    expect(resizedWidth).toBe(111);
    const expected=operation==='P'?Array.from({length:count},(_,i)=>111-count+i):[];
    expect(small.blue,operation+' '+count+' at the visible right edge').toEqual(expected);
    expect(expanded.blue,operation+' '+count+' after expanding').toEqual(expected);
    for(const state of [small,expanded]){
      expect(state.unicode).toBe('界');expect(state.foreground).toBe(179);expect(state.following).toBe('a');
    }
  }
});

test('d cycles the five approved palettes from entities and Mindset',async({page})=>{
  await exposeMountainTerminal(page);await ready(page);
  let sockets=0;page.on('websocket',()=>sockets++);
  await page.keyboard.press('End');await page.keyboard.press('Tab');
  await expect(screen(page)).toContainText('d: colors');
  const colors=()=>page.evaluate(()=>{
    const term=window.mountainTerminal;
    const lines=Array.from({length:term.rows},(_,y)=>term.buffer.active.getLine(term.buffer.active.viewportY+y));
    const event=lines.findIndex(line=>/^ Events\s/.test(line?.translateToString(true)||''));
    let mountain=false,events=false;
    for(let y=4;y<term.rows-4;y++)for(let x=0;x<term.cols;x++){
      const cell=lines[y]?.getCell(x);
      if(!/[\u2801-\u28ff]/.test(cell?.getChars()||''))continue;
      if(y<event-1&&[37,179].includes(cell.getFgColor()))mountain=true;
      if(y>event&&x<24&&cell.getFgColor()===62)events=true;
    }
    return{mountain,events};
  });
  for(const panel of ['entities','Mindset']){
    await expect.poll(colors).toEqual({mountain:true,events:true});
    for(const expected of [{mountain:false,events:false},{mountain:false,events:false},{mountain:true,events:false},{mountain:false,events:true},{mountain:true,events:true}]){
      await page.keyboard.press('d');await expect.poll(colors).toEqual(expected);
      await expect(screen(page)).not.toContainText('normal boost');
    }
    if(panel==='entities'){await page.keyboard.press('Tab');await expect(screen(page)).toContainText('Enter: open');}
  }
  expect(sockets).toBe(0); // No replacement connection while changing presentation.
});

test('rapid resize and day movement leave one marker and no mountain ink over cards', async ({page}, testInfo) => {
  await exposeMountainTerminal(page);
  await ready(page);
  await page.evaluate(()=>{window.resizeFitProbe=new FitAddon.FitAddon();window.mountainTerminal.loadAddon(window.resizeFitProbe);});
  await page.keyboard.press('Home'); await page.keyboard.press('ArrowUp');
  await expect(screen(page)).toContainText('1 Total');
  const snapshot = () => page.evaluate(() => {
    const term = window.mountainTerminal;
    const fitted=window.resizeFitProbe.proposeDimensions();
    const lines = Array.from({length:term.rows},(_,y)=>term.buffer.active.getLine(term.buffer.active.viewportY+y));
    const text = lines.map(line=>line?.translateToString(true)||'');
    const heading = text.findIndex(line=>/^ Events\s/.test(line));
    const footer = text.findLastIndex(line=>/^─+$/.test(line));
    const markers = Array.from({length:term.cols},(_,x)=>x).filter(x=>lines[heading-1]?.getCell(x)?.getBgColor()===62);
    const overlap=[];
    for(let y=heading+1;y<term.rows-4;y++) for(let x=24;x<term.cols;x++) if(/[\u2801-\u28ff]/.test(lines[y]?.getCell(x)?.getChars()||'')) overlap.push({x,y});
    const geometry={columns:term.cols-fitted.cols,rows:term.rows-fitted.rows,footerRow:footer-(term.rows-4),footerWidth:(text[footer]?.length??-1)-term.cols};
    return {geometry,date:text[heading]?.match(/^ Events\s+[A-Z][a-z]+ \d+, \d{4}/)?.[0],cols:term.cols,rows:term.rows,heading,markers,overlap,complete:fitted?.cols===term.cols&&fitted?.rows===term.rows&&footer===term.rows-4&&text[footer]?.length===term.cols,text};
  });
  const original=(await snapshot()).date;
  // Reproduce the dense diary failure: narrow/taller to wider/shorter,
  // then move the day marker across the newly added columns.
  for(const size of [{width:960,height:450},{width:480,height:480},{width:960,height:400}]) {
    await page.setViewportSize(size);
    await expect.poll(async()=>{const s=await snapshot();return s.complete&&s.cols<(size.width===480?60:120)&&s.rows<30;}).toBe(true);
    await expect.poll(async()=>{const s=await snapshot();return s.markers.length===1&&s.markers[0]===s.cols-1;}).toBe(true);
    for(let step=0;step<45;step++) await page.keyboard.press('ArrowLeft');
    await expect.poll(async()=>(await snapshot()).markers).toEqual([0]);
    for(let step=0;step<45;step++) await page.keyboard.press('ArrowRight');
    await expect.poll(async()=>{const s=await snapshot();return s.markers.length===1&&s.markers[0]===s.cols-1;}).toBe(true);
  }
  for(let round=0;round<8;round++) {
    for(const size of [{width:960,height:400},{width:480,height:480},{width:1200,height:850},{width:960,height:450}]) {
      await page.setViewportSize(size);
      await page.keyboard.press('ArrowLeft'); await page.keyboard.press('ArrowRight');
    }
    let accepted;
    await expect.poll(async()=>{
      accepted=await snapshot();
      return {geometry:accepted.geometry,date:accepted.date,markerOffsets:accepted.markers.map(x=>x-(accepted.cols-1)),overlap:accepted.overlap};
    }).toEqual({geometry:{columns:0,rows:0,footerRow:0,footerWidth:0},date:original,markerOffsets:[0],overlap:[]});
    await testInfo.attach('resize-'+round,{body:JSON.stringify(accepted),contentType:'application/json'});
  }
  if((await screen(page).textContent()).includes('spider-demo')) {
    await page.keyboard.press('Escape');await page.keyboard.press('End');
    await page.setViewportSize({width:480,height:850});
    await page.keyboard.press('Tab');await page.keyboard.press('Tab');
    await page.keyboard.press('End');
    await expect.poll(async()=>(await snapshot()).heading).toBeGreaterThan(16);
    await page.setViewportSize({width:1200,height:850});
    await expect.poll(async()=>{const s=await snapshot();return s.complete&&s.cols>120&&s.heading>16;}).toBe(true);
    await page.keyboard.press('Tab');
    await expect.poll(async()=>(await snapshot()).heading).toBeLessThan(16);
    await expect.poll(async()=>(await snapshot()).overlap).toEqual([]);
    await page.screenshot({path:testInfo.outputPath('grown-then-collapsed-mindset.png')});
  }

});

test('ordinary terminal editing survives repeated shrink and grow in both screens',async({page})=>{
 await ready(page);
 const states=await page.evaluate(async()=>{
  const states=[];
  for(const alternate of [false,true]){
   const host=document.createElement('div');document.body.append(host);
   const term=new Terminal({cols:120,rows:20,allowProposedApi:true,scrollback:20,convertEol:true});term.open(host);
   const write=text=>new Promise(resolve=>term.write(text,resolve));
   try{
    if(alternate)await write('\x1b[?1049h');
    await write('\x1b[H\x1b[38;5;179m界e\u0301\x1b[m');
    for(let cycle=0;cycle<4;cycle++)for(const cols of [111,40,80,120]){
     term.resize(cols,20);
     const prefix=term.buffer.active.getLine(0).translateToString(true);
     await write('\x1b[5;1Habcde\r\x1b[2C\x1b[P\x1b[@X');
     const edited=term.buffer.active.getLine(4).translateToString(true);
     await write(`\x1b[7;${cols}HAB`);
     const last=term.buffer.active.getLine(6).getCell(cols-1).getChars();
     const wrapped=term.buffer.active.getLine(7).getCell(0).getChars();
     await write('\x1b[7;1H\x1b[2K\x1b[8;1H\x1b[2K');
     states.push({alternate,cols,prefix,edited,last,wrapped,width:term.buffer.active.getLine(4).length,color:term.buffer.active.getLine(0).getCell(0).getFgColor()});
    }
    if(alternate)await write('\x1b[?1049l');
   }finally{term.dispose();host.remove();}
  }
  return states;
 });
 for(const state of states){
  expect(state.prefix).toBe('界é');expect(state.color).toBe(179);
  expect(state.edited).toBe('abXde');expect(state.width).toBe(state.cols);
  expect(state.last).toBe('A');expect(state.wrapped).toBe('B');
 }
});


test('native renderer clears old mountain cells and marker after a grow',async({page})=>{
 const frames=JSON.parse(readFileSync(process.env.THOUGHTS_RENDERER_RESIZE_FRAMES,'utf8'));
 await ready(page);
 const results=await page.evaluate(async frames=>{
  const results=[];
  for(const fixedSize of [false,true]){
   const host=document.createElement('div');document.body.append(host);
   const term=new Terminal({cols:55,rows:26,allowProposedApi:true,scrollback:0,convertEol:true});term.open(host);
   const write=text=>new Promise(resolve=>term.write(text,resolve));
   try{
    // Fixed-size replay isolates the native diff from browser resize ordering.
    if(fixedSize){term.resize(111,21);await write('\x1b[?1049h');}
    for(const frame of fixedSize?frames.slice(1):frames){
     if(!fixedSize)term.resize(frame.cols,frame.rows);
     await write(frame.output);
    }
    const lines=Array.from({length:21},(_,y)=>term.buffer.active.getLine(y));
    const stale=[],markers=[];
    for(let y=0;y<21;y++)for(let x=0;x<111;x++){
     const cell=lines[y]?.getCell(x);
     if(y>=10&&/[\u2801-\u28ff]/.test(cell?.getChars()||''))stale.push({x,y});
     if(cell?.getBgColor()===62)markers.push({x,y});
    }
    results.push({fixedSize,stale,markers,heading:lines[9].translateToString(true),card:lines[11].translateToString(true)});
   }finally{term.dispose();host.remove();}
  }
  return results;
 },frames);
 for(const result of results){
  expect(result.heading).toContain('Events');expect(result.card).toContain('Event card');
  expect(result.stale).toEqual([]);expect(result.markers).toEqual([{x:110,y:8}]);
 }
});
