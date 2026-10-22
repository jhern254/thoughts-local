// A decorative activity cue, never an encoding-progress measurement. The fixed
// curve is rendered once; local transforms move the light without text or
// transport updates.
class ThoughtsPreparationWave {
  constructor(element) {
    this.element = element;
    this.trace = element.querySelector('pre');
    // A 78 by 6 braille grid holds 156 by 24 dots. Two complete sine cycles
    // meet the center axis at both edges, regardless of the displayed width.
    const grid = new Uint8Array(78 * 6);
    // Unicode braille's bottom row uses bits 6 and 7; the upper three rows
    // occupy bits 0–2 in the left column and 3–5 in the right column.
    const bits = [[1, 2, 4, 64], [8, 16, 32, 128]];
    for (let x = 0; x < 156; x++) {
      const y = x === 0 || x === 155 ? 12 : Math.round(12 - 10 * Math.sin(x / 155 * Math.PI * 4));
      grid[Math.floor(y / 4) * 78 + Math.floor(x / 2)] |= bits[x % 2][y % 4];
    }
    const lines = [];
    for (let row = 0; row < 6; row++) {
      lines.push(Array.from(grid.subarray(row * 78, (row + 1) * 78), mask => String.fromCharCode(0x2800 + mask)).join(''));
    }
    for (const trace of element.querySelectorAll('pre')) trace.textContent = lines.join('\n');
  }
  setPlayback(active, visible, reducedMotion) {
    this.element.dataset.animating = String(active && visible && !reducedMotion);
  }
}
