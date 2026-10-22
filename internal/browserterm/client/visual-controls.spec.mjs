import {test, expect} from '@playwright/test';
import {clickControl} from './visual-helpers.mjs';

test('Visual mouse controls wait for terminal layout to settle before clicking', async ({page}) => {
  // Resizing and the preparation indicator move the terminal independently
  // of its text updates. A one-time coordinate sample can hit the old position.
  await page.setContent('<div class="xterm-screen" style="position:fixed;left:40px;top:40px"><div class="xterm-rows" style="pointer-events:none"><span>Apply</span></div></div>');
  await page.locator('.xterm-screen').evaluate(terminal => {
    terminal.addEventListener('click', () => { terminal.dataset.clicked = 'true'; });
    const motion = terminal.animate([{transform:'translateX(-1000px)'}, {transform:'translateX(0)'}], {duration:400, fill:'forwards'});
    // xterm also replaces text nodes as terminal output arrives during layout.
    function redraw() {
      terminal.firstElementChild.innerHTML = '<span>Apply</span>';
      if (motion.playState !== 'finished') requestAnimationFrame(redraw);
    }
    requestAnimationFrame(redraw);
  });
  await clickControl(page, 'Apply');
  await expect(page.locator('.xterm-screen')).toHaveAttribute('data-clicked', 'true');
});
