import {test, expect} from '@playwright/test';
import {clickControl} from './visual-helpers.mjs';

test('terminal control clicks retry a label replaced during measurement', async ({page}) => {
  await page.setContent('<div class="xterm-screen"><div class="xterm-rows"><span>Cancel upload</span></div></div>');
  await page.evaluate(() => {
    window.controlClicks = 0;
    window.labelRedraws = 0;
    document.querySelector('.xterm-screen').addEventListener('click', event => {
      if (event.target.textContent === 'Cancel upload') window.controlClicks++;
    });
    // Reproduce xterm replacing the measured span before its terminal ancestor
    // is read, without relying on browser scheduling to hit the redraw race.
    const measureRange = Range.prototype.getBoundingClientRect;
    Range.prototype.getBoundingClientRect = function () {
      const bounds = measureRange.call(this);
      const label = document.querySelector('.xterm-rows span');
      label.replaceWith(label.cloneNode(true));
      window.labelRedraws++;
      Range.prototype.getBoundingClientRect = measureRange;
      return bounds;
    };
  });

  await clickControl(page, 'Cancel upload');

  expect(await page.evaluate(() => window.labelRedraws)).toBe(1);
  expect(await page.evaluate(() => window.controlClicks)).toBe(1);
});
