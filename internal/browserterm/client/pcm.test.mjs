import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source = await readFile(new URL('../static/pcm-worklet.js', import.meta.url), 'utf8');
function processorAt(sampleRateHz) {
  let Processor;
  const frames = [];
  class AudioWorkletProcessor {
    constructor() { this.port = {postMessage: message => frames.push(message)}; }
  }
  vm.runInNewContext(source, {AudioWorkletProcessor, sampleRate: sampleRateHz, registerProcessor: (_, implementation) => { Processor = implementation; }});
  const processor = new Processor();
  processor.port.onmessage({data: {credit: 4}});
  return {processor, frames};
}
test('PCM16 uses explicit signed little endian encoding', () => {
  const {processor, frames} = processorAt(16000);
  const samples = new Float32Array(1600); samples[0] = -1; samples[1] = 1;
  processor.process([[samples]], []);
  const bytes = new Uint8Array(frames[0].pcm);
  assert.deepEqual([...bytes.slice(0, 4)], [0, 128, 255, 127]);
  assert.equal(bytes.length, 3200);
});
for (const sampleRateHz of [16000, 44100, 48000]) {
  test(`continuous ${sampleRateHz} Hz conversion survives uneven rendering blocks`, () => {
    const {processor, frames} = processorAt(sampleRateHz);
    const sourceSamples = new Float32Array(sampleRateHz / 10).fill(0.25);
    for (let offset = 0; offset < sourceSamples.length; offset += 127) processor.process([[sourceSamples.subarray(offset, offset + 127)]], []);
    assert.equal(frames.length, 1);
    const samples = new DataView(frames[0].pcm);
    for (let offset = 0; offset < samples.byteLength; offset += 2) assert.equal(samples.getInt16(offset, true), 8192);
  });
}
test('credit exhaustion stops instead of growing MessagePort audio buffering', () => {
  const {processor, frames} = processorAt(16000);
  for (let chunk = 0; chunk < 100; chunk++) processor.process([[new Float32Array(1600)]], []);
  assert.equal(frames.filter(frame => frame.pcm).length, 4);
  assert.equal(frames.filter(frame => frame.status === 'failed').length, 1);
});
test('Stop and invalid samples release the private partial frame', () => {
  for (const invalid of [NaN, Infinity]) {
    const {processor, frames} = processorAt(16000);
    processor.process([[new Float32Array([0.25, invalid])]], []);
    assert.equal(frames[0].status, 'failed');
    assert.equal(processor.pcmFrameStorage, undefined);
  }
  const {processor} = processorAt(16000);
  processor.process([[new Float32Array([0.25])]], []);
  const partial = processor.pcmFrameStorage;
  processor.port.onmessage({data: {stop: true}});
  assert.ok(new Uint8Array(partial).every(byte => byte === 0));
  assert.equal(processor.process([[new Float32Array(1600)]], []), false);
});

test('a 750ms main-thread pause preserves ordered PCM and recovers on returned credit', () => {
  const {processor, frames} = processorAt(16000);
  processor.port.onmessage({data: {credit: 10}});
  // No main-thread acknowledgements arrive during these eight 100ms frames.
  for (let frame = 0; frame < 8; frame++) {
    processor.process([[new Float32Array(1600).fill((frame + 1) / 16)]], []);
  }
  assert.equal(frames.filter(frame => frame.pcm).length, 8);
  assert.equal(frames.some(frame => frame.status === 'failed'), false);
  for (let index = 0; index < 8; index++) {
    assert.equal(new DataView(frames[index].pcm).getInt16(0, true), (index + 1) * 2048);
    processor.port.onmessage({data: {credit: 1}});
  }
  const previousFrames = frames.length;
  for (let frame = 0; frame < 10; frame++) processor.process([[new Float32Array(1600)]], []);
  assert.equal(frames.length - previousFrames, 10);
  assert.equal(processor.active, true);
});

test('excessive credit cannot exceed one second of queued PCM', () => {
  const {processor, frames} = processorAt(16000);
  processor.port.onmessage({data: {credit: 1000000}});
  for (let frame = 0; frame < 100; frame++) processor.process([[new Float32Array(1600)]], []);
  assert.equal(frames.filter(frame => frame.pcm).length, 10);
  assert.equal(frames.filter(frame => frame.status === 'failed').length, 1);
  assert.equal(processor.pcmFrameStorage, undefined);
});
