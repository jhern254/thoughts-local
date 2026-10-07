import {test, expect} from '@playwright/test';

const screen = page => page.locator('.xterm-rows');
async function ready(page) {
  await page.goto('/');
  await expect(page.locator('#status')).toHaveText('Connected');
  await expect(screen(page)).toContainText('Events');
}

test('renders newlines at the same column as native terminal output', async ({page}) => {
  await ready(page);
  // A pipe does not apply the PTY's ONLCR output translation. Without the
  // corresponding xterm option, each new line incorrectly moves to the right.
  await expect.poll(() => screen(page).locator('div').nth(2).textContent()).toMatch(/^ Events/);
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
      // The backend divider precedes two navigation rows and the final blank
      // row. Summing wrapped divider rows also catches stale backend widths.
      const divider = rows.findIndex(row => /^─+$/.test(row));
      return {cols: term.cols, rows: term.rows, dividerWidth: rows.filter(row => /^─+$/.test(row)).reduce((sum, row) => sum + row.length, 0), divider};
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
