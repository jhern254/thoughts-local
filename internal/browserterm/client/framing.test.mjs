import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

const source = await readFile(new URL('../static/framing.js', import.meta.url), 'utf8');
const framing = vm.createContext({});
vm.runInContext(source, framing);
const settings = {fit:'fill', zoom:100, positionX:5000, positionY:5000};

test('Fill crops a centered portrait while Fit preserves the complete image', () => {
  const fill = framing.backgroundGeometry(600, 1200, 1200, 800, settings);
  assert.deepEqual({...fill}, {width:1200, height:2400, left:0, top:-800});
  const fit = framing.backgroundGeometry(600, 1200, 1200, 800, {...settings,fit:'fit',zoom:300,positionX:0});
  assert.deepEqual({...fit}, {width:400, height:800, left:400, top:0});
});
test('preview and viewport expose the same crop for every shape and zoom', () => {
  for (const [width,height] of [[600,1200],[1200,1200],[4000,500],[12,8]]) {
    for (const zoom of [100,175,300]) {
      for (const position of [0,5000,10000]) {
        const values = {...settings,zoom,positionX:position,positionY:position};
        const full = framing.backgroundGeometry(width,height,1200,800,values);
        const preview = framing.backgroundGeometry(width,height,120,80,values);
        for (const key of ['width','height','left','top']) assert.ok(Math.abs(preview[key]*10-full[key]) < 0.00001);
        assert.ok(full.left <= 0 && full.top <= 0);
        assert.ok(full.left+full.width >= 1200 && full.top+full.height >= 800);
      }
    }
  }
});
test('malformed stored framing uses safe centered defaults', () => {
  for (const value of [undefined,{}, {fit:'other',zoom:100,positionX:5000,positionY:5000}, {...settings,zoom:Infinity}, {...settings,positionX:-1}]) {
    assert.deepEqual({...framing.safeBackgroundFraming(value)},settings);
  }
});
