import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

function activationFixture() {
  const pending = [], revoked = [], published = [];
  let fetches = 0;
  const context = vm.createContext({
    window: {innerWidth: 1200, innerHeight: 850},
    URL: {createObjectURL: () => `blob:${fetches}`, revokeObjectURL: value => revoked.push(value)},
    fetch: async () => { fetches++; return {ok: true, blob: async () => ({})}; },
    Image: class {
      constructor() { this.style = {}; this.naturalWidth = 1200; this.naturalHeight = 850; }
      decode() { return new Promise(resolve => pending.push(resolve)); }
      replaceWith(image) { published.push(image.src); }
      removeAttribute() {}
    }
  });
  vm.runInContext(readFileSync(new URL('../static/framing.js', import.meta.url), 'utf8'), context);
  vm.runInContext(readFileSync(new URL('../static/visual.js', import.meta.url), 'utf8'), context);
  const visual = vm.runInContext('Object.create(ThoughtsVisual.prototype)', context);
  Object.assign(visual, {
    connectionGeneration: 0, activationGeneration: 0,
    saved: {background: false}, image: new context.Image(),
    surface: {}, overlay: {style: {}}, terminal: {options: {theme: {}}}
  });
  visual.updateDarkness = () => {};
  const settings = backgroundID => ({background: true, backgroundID, darkness: 70});
  return {visual, context, pending, revoked, published, settings, fetches: () => fetches};
}

test('same background settings reuse the decoded image', async () => {
  const f = activationFixture();
  const first = f.visual.activate(f.settings('first'));
  await new Promise(resolve => setImmediate(resolve)); f.pending.shift()(); await first;
  const image = f.visual.image;
  const changed = f.visual.activate({...f.settings('first'), darkness: 30});
  await new Promise(resolve => setImmediate(resolve));
  // Finish an unexpected decode too, so the regression reports the extra work.
  if (f.pending.length) f.pending.shift()();
  await changed;
  assert.equal(f.fetches(), 1);
  assert.equal(f.visual.image, image);
  assert.equal(f.visual.saved.darkness, 30);
});

test('a newer background wins over a delayed decode and releases its URL', async () => {
  const f = activationFixture();
  const first = f.visual.activate(f.settings('first'));
  await new Promise(resolve => setImmediate(resolve));
  const second = f.visual.activate(f.settings('second'));
  await new Promise(resolve => setImmediate(resolve));
  f.pending[1](); await second; f.pending[0](); await first;
  assert.equal(f.visual.saved.backgroundID, 'second');
  assert.deepEqual(f.published, ['blob:2']);
  assert.deepEqual(f.revoked, ['blob:1']);
});

test('a disconnected operation cannot publish its candidate', async () => {
  const f = activationFixture();
  const activation = f.visual.activate(f.settings('first'));
  await new Promise(resolve => setImmediate(resolve));
  f.visual.connectionGeneration++;
  f.pending.shift()(); await activation;
  assert.equal(f.visual.saved.background, false);
  assert.deepEqual(f.published, []);
  assert.deepEqual(f.revoked, ['blob:1']);
});

test('late restore metadata cannot overwrite a newer activation', async () => {
  const f = activationFixture();
  const fetch = f.context.fetch;
  let release;
  f.context.fetch = path => path === '/visual' ? new Promise(resolve => { release = resolve; }) : fetch(path);
  const restore = f.visual.restore();
  const newer = f.visual.activate(f.settings('newer'));
  await new Promise(resolve => setImmediate(resolve));
  f.pending.shift()(); await newer;
  release({ok:true,json:async()=>f.settings('older')}); await restore;
  assert.equal(f.visual.saved.backgroundID,'newer');
  assert.equal(f.fetches(),1);
});

test('connection retry restores once for the accepted generation', () => {
  const f = activationFixture();
  f.visual.restoreGeneration=0;
  let restores=0;
  f.visual.restore=()=>{restores++;return Promise.resolve();};
  f.visual.restoreConnection();
  f.visual.connectionGeneration++;
  f.visual.restoreConnection(); f.visual.restoreConnection();
  assert.equal(restores,1);
});

test('failed candidate decode leaves the working background intact', async () => {
  const f = activationFixture();
  const first=f.visual.activate(f.settings('first'));
  await new Promise(resolve=>setImmediate(resolve)); f.pending.shift()(); await first;
  const image=f.visual.image;
  f.context.Image.prototype.decode=async()=>{throw new Error('synthetic decode failure');};
  await assert.rejects(f.visual.activate(f.settings('second')));
  assert.equal(f.visual.image,image);
  assert.equal(f.visual.saved.backgroundID,'first');
  assert.deepEqual(f.revoked,['blob:2']);
});
