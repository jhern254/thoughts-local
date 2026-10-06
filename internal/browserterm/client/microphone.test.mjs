import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

const source = await readFile(new URL('../static/microphone.js', import.meta.url), 'utf8');
function setup() {
  const events = [], recorders = [], timers = new Map();
  let now = 0, id = 0, stopped = 0;
  let resolvePermission;
  const stream = {getTracks: () => [{stop: () => { stopped++; }}]};
  class Recorder {
    static isTypeSupported() { return true; }
    constructor() { this.state = 'inactive'; recorders.push(this); }
    start(interval) { this.interval = interval; this.state = 'recording'; }
    stop() { this.state = 'inactive'; } // Tests explicitly deliver final stop.
  }
  const sandbox = {navigator: {mediaDevices: {getUserMedia: () => new Promise(resolve => { resolvePermission = resolve; })}}, MediaRecorder: Recorder,
    performance: {now: () => now}, setTimeout: (fn, delay) => {timers.set(++id, {fn, delay}); return id;}, clearTimeout: key => timers.delete(key)};
  vm.runInNewContext(source, sandbox);
  const mic = new sandbox.ThoughtsMicrophone(action => events.push(action));
  return {mic, events, recorders, timers, stream, grant: () => resolvePermission(stream), setNow: value => {now = value;}, stopped: () => stopped};
}

test('recording delivers chunks incrementally and runs past two minutes until twenty', async () => {
  const s = setup();
  const start = s.mic.start(); s.grant(); await start;
  const recorder = s.recorders[0];
  assert.equal(recorder.interval, 1000);
  s.setNow(3 * 60 * 1000);
  recorder.ondataavailable({data: {size: 100}});
  assert.equal(recorder.state, 'recording');
  for (let i = 0; i < 1200; i++) recorder.ondataavailable({data: {size: 16000}});
  assert.equal(s.mic.chunks, undefined);
  assert.equal(s.mic.audio, undefined);
  assert.equal([...s.timers.values()][0].delay, 20 * 60 * 1000);
  s.setNow(20 * 60 * 1000);
  recorder.ondataavailable({data: {size: 100}});
  assert.equal(recorder.state, 'inactive');
  assert.equal(s.stopped(), 1);
  recorder.onstop();
  assert.deepEqual(s.events, ['recording', 'stop', 'stopped']);
  assert.equal(s.timers.size, 0);
});
test('stop completes once and an explicit restart owns fresh capture', async () => {
  const s = setup();
  const first = s.mic.start(); s.grant(); await first;
  s.mic.stop(); s.mic.stop();
  s.recorders[0].onstop();
  const second = s.mic.start(); s.grant(); await second;
  s.recorders[0].onerror();
  assert.equal(s.recorders[1].state, 'recording');
  assert.deepEqual(s.events, ['recording', 'stop', 'stopped', 'recording']);
  s.mic.cancel();
  assert.equal(s.stopped(), 2);
});
test('late permission after cancellation stops tracks without starting recorder', async () => {
  const s = setup(); const start = s.mic.start();
  s.mic.cancel(); s.grant(); await start;
  assert.equal(s.stopped(), 1);
  assert.equal(s.recorders.length, 0);
  assert.deepEqual(s.events, []);
});
test('stop while permission is pending releases the draft without waiting', async () => {
  const s = setup(); const start = s.mic.start();
  s.mic.stop();
  assert.deepEqual(s.events, ['stop', 'stopped']);
  s.grant(); await start;
  assert.equal(s.stopped(), 1);
  assert.equal(s.recorders.length, 0);
});
test('cancel clears the deadline and makes recorder callbacks harmless', async () => {
  const s = setup(); const start = s.mic.start(); s.grant(); await start;
  s.mic.cancel();
  s.recorders[0].onstop(); s.recorders[0].onerror();
  assert.equal(s.timers.size, 0);
  assert.deepEqual(s.events, ['recording']);
  assert.equal(s.stopped(), 1);
});
test('deadline fires even without data callbacks', async () => {
  const s = setup(); const start = s.mic.start(); s.grant(); await start;
  s.setNow(20 * 60 * 1000);
  [...s.timers.values()][0].fn();
  s.recorders[0].onstop();
  assert.deepEqual(s.events, ['recording', 'stop', 'stopped']);
  assert.equal(s.stopped(), 1);
});
test('duplicate Start does not acquire another microphone', async () => {
  const s = setup(); const start = s.mic.start(); await s.mic.start();
  s.grant(); await start;
  await s.mic.start();
  assert.equal(s.recorders.length, 1);
  s.mic.cancel();
});
test('permission failures produce fixed categories and permit retry', async () => {
  const actions = [];
  const sandbox = {navigator: {mediaDevices: {getUserMedia: async () => {throw {name: 'NotAllowedError', message: 'PRIVATE-ERROR'};}}}, MediaRecorder: class {}, clearTimeout: () => {}};
  vm.runInNewContext(source, sandbox);
  const mic = new sandbox.ThoughtsMicrophone(action => actions.push(action));
  await mic.start(); await mic.start();
  assert.deepEqual(actions, ['denied', 'denied']);
});
