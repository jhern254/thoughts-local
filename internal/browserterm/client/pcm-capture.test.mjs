import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source = await readFile(new URL('../static/pcm-capture.js', import.meta.url), 'utf8');
const grant = {sessionID: '11'.repeat(16), draftID: 1, recordingID: 2, audioCapability: '22'.repeat(32)};
function setup() {
  const events = [], sockets = [], processors = [], timers = new Map();
  let resolvePermission, resolveSocket, stoppedTracks = 0, closedContexts = 0;
  const socketCreated = new Promise(resolve => { resolveSocket = resolve; });
  class AudioContext {
    constructor() { this.sampleRate = 16000; this.audioWorklet = {addModule: async () => {}}; }
    resume() { return Promise.resolve(); }
    close() { closedContexts++; return Promise.resolve(); }
    createMediaStreamSource() { return {connect() {}, disconnect() {}}; }
    get destination() { return {}; }
  }
  class AudioWorkletNode {
    constructor() { this.port = {postMessage() {}, close() {}}; processors.push(this); }
    connect() {} disconnect() {}
  }
  class WebSocket {
    static OPEN = 1;
    constructor(address) { this.address = address; this.sent = []; this.bufferedAmount = 0; sockets.push(this); resolveSocket(this); }
    send(frame) { this.sent.push(new Uint8Array(frame)); }
    close() { this.readyState = 3; this.onclose?.(); }
  }
  const sandbox = {navigator: {mediaDevices: {getUserMedia: () => new Promise(resolve => { resolvePermission = resolve; })}},
    AudioContext, AudioWorkletNode, WebSocket, location: {protocol: 'http:', host: '127.0.0.1:7777'},
    setTimeout: callback => { const token = {}; timers.set(token, callback); return token; }, clearTimeout: token => timers.delete(token)};
  vm.runInNewContext(source, sandbox);
  const capture = new sandbox.ThoughtsPCMRecording((status, recording) => events.push({status, recording}));
  const stream = {getTracks: () => [{stop: () => { stoppedTracks++; }}]};
  return {capture, events, sockets, processors, timers, socketCreated, grantPermission: () => resolvePermission(stream),
    stopped: () => stoppedTracks, closed: () => closedContexts};
}
async function startRecording(fixture) {
  fixture.capture.prepare();
  const started = fixture.capture.start(grant);
  fixture.grantPermission();
  const socket = await fixture.socketCreated;
  socket.readyState = 1; socket.onopen();
  socket.onmessage({data: new Uint8Array([84, 65, 1, 3, 0]).buffer});
  await started;
  return socket;
}
test('capability is binary handshake metadata and PCM uses only dedicated socket', async () => {
  const fixture = setup(); const socket = await startRecording(fixture);
  assert.equal(socket.address, 'ws://127.0.0.1:7777/voice/audio');
  assert.equal(socket.sent[0].byteLength, 36);
  const pcm = new ArrayBuffer(3200);
  fixture.processors[0].port.onmessage({data: {pcm}});
  assert.equal(socket.sent[1].byteLength, 3200);
  assert.deepEqual(socket.sent[0].slice(4), new Uint8Array(32).fill(34));
  assert.ok(new Uint8Array(pcm).every(byte => byte === 0));
  fixture.capture.stop();
  assert.deepEqual(fixture.events.map(event => event.status), ['recording', 'stop', 'stopped']);
  assert.equal(fixture.stopped(), 1); assert.equal(fixture.closed(), 1);
});
test('outbound overflow stops capture without replay or growth', async () => {
  const fixture = setup(); const socket = await startRecording(fixture);
  socket.bufferedAmount = 48000;
  fixture.processors[0].port.onmessage({data: {pcm: new ArrayBuffer(3200)}});
  assert.equal(socket.sent.length, 1);
  assert.equal(fixture.events.at(-1).status, 'failed');
  assert.equal(fixture.stopped(), 1);
});
test('late permission and worklet callbacks cannot revive a canceled attempt', async () => {
  const fixture = setup(); fixture.capture.prepare();
  const started = fixture.capture.start(grant);
  fixture.capture.stop(); fixture.grantPermission(); await started;
  assert.equal(fixture.stopped(), 1); assert.equal(fixture.sockets.length, 0);
  assert.equal(fixture.closed(), 1);
});
test('Stop clears deadlines and closes each owned resource once', async () => {
  const fixture = setup(); const socket = await startRecording(fixture);
  const processor = fixture.processors[0];
  fixture.capture.stop(); fixture.capture.stop(); fixture.capture.cancel();
  processor.port.onmessage({data: {pcm: new ArrayBuffer(3200)}});
  assert.equal(socket.sent.length, 1); assert.equal(fixture.timers.size, 0);
  assert.equal(fixture.closed(), 1);
});
test('accepted Start preserves the AudioContext activated by the trusted key event', async () => {
  const fixture = setup();
  fixture.capture.prepare();
  fixture.capture.cancel(true);
  assert.equal(fixture.closed(), 0);
  await startRecording(fixture);
  fixture.capture.stop();
  assert.equal(fixture.closed(), 1);
});
test('failed socket writes clear PCM before releasing capture', async () => {
  const fixture = setup(); const socket = await startRecording(fixture);
  let sentPCM;
  socket.send = frame => { sentPCM = frame; throw new Error('PRIVATE-WRITE-ERROR'); };
  const pcm = new ArrayBuffer(3200); new Uint8Array(pcm).fill(85);
  fixture.processors[0].port.onmessage({data: {pcm}});
  assert.ok(sentPCM.every(byte => byte === 0));
  assert.ok(new Uint8Array(pcm).every(byte => byte === 0));
  assert.equal(fixture.events.at(-1).status, 'failed');
  assert.equal(fixture.stopped(), 1);
});
test('handshake failure emits fixed state and clears the capability frame', async () => {
  const fixture = setup(); fixture.capture.prepare();
  const started = fixture.capture.start(grant);
  fixture.grantPermission();
  const socket = await fixture.socketCreated;
  socket.readyState = 1;
  let handshake;
  socket.send = frame => { handshake = frame; throw new Error('PRIVATE-HANDSHAKE-ERROR'); };
  socket.onopen();
  await started;
  assert.ok(handshake.every(byte => byte === 0));
  assert.deepEqual(fixture.events.map(event => event.status), ['failed']);
  assert.equal(fixture.stopped(), 1);
});

test('a non fixed worklet frame stops capture before sending audio', async () => {
  const fixture = setup(); const socket = await startRecording(fixture);
  const pcm = new ArrayBuffer(3198); new Uint8Array(pcm).fill(85);
  fixture.processors[0].port.onmessage({data: {pcm}});
  assert.equal(socket.sent.length, 1);
  assert.ok(new Uint8Array(pcm).every(byte => byte === 0));
  assert.equal(fixture.events.at(-1).status, 'failed');
  assert.equal(fixture.stopped(), 1);
});
