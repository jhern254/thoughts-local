/* PCM16LE wire format and ownership are described in docs/browser-audio.md. */
class ThoughtsPCMProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.active = true;
    this.chunkCredits = 0;
    this.sourceSamplesPerOutputSample = sampleRate / 16000;
    this.remainingOutputWidth = this.sourceSamplesPerOutputSample;
    this.weightedAmplitude = 0;
    this.pcmSampleIndex = 0;
    this.pcmFrameStorage = new ArrayBuffer(1600 * 2);
    this.pcmFrameView = new DataView(this.pcmFrameStorage);
    this.port.onmessage = event => {
      if (event.data.stop) this.release();
      else if (Number.isInteger(event.data.credit) && event.data.credit > 0) this.chunkCredits = Math.min(4, this.chunkCredits + event.data.credit);
    };
    if (sampleRate < 16000 || !Number.isFinite(sampleRate)) this.fail();
  }
  release() {
    this.active = false;
    if (this.pcmFrameStorage) new Uint8Array(this.pcmFrameStorage).fill(0);
    this.pcmFrameStorage = undefined;
    this.pcmFrameView = undefined;
    this.weightedAmplitude = 0;
  }
  fail() {
    if (!this.active) return;
    this.release();
    this.port.postMessage({status: 'failed'});
  }
  writeSample(amplitude) {
    const normalizedAmplitude = Math.max(-1, Math.min(1, amplitude));
    const signedSample = Math.round(normalizedAmplitude * (normalizedAmplitude < 0 ? 32768 : 32767));
    this.pcmFrameView.setInt16(this.pcmSampleIndex * 2, signedSample, true);
    if (++this.pcmSampleIndex !== 1600) return;
    if (this.chunkCredits === 0) { this.fail(); return; }
    this.chunkCredits--;
    this.port.postMessage({pcm: this.pcmFrameStorage}, [this.pcmFrameStorage]);
    this.pcmFrameStorage = new ArrayBuffer(1600 * 2);
    this.pcmFrameView = new DataView(this.pcmFrameStorage);
    this.pcmSampleIndex = 0;
  }
  process(inputs) {
    if (!this.active) return false;
    const channels = inputs[0];
    if (!channels?.length) return true;
    if (channels.length !== 1) { this.fail(); return false; }
    // Average the source waveform over each output sample's time interval.
    // Fractional widths carry across rendering blocks, including 44.1 kHz
    // input. Merely changing a sample-rate field would change pitch/duration.
    for (const amplitude of channels[0]) {
      if (!Number.isFinite(amplitude)) { this.fail(); break; }
      let remainingSourceWidth = 1;
      while (remainingSourceWidth > 1e-9 && this.active) {
        const overlapWidth = Math.min(remainingSourceWidth, this.remainingOutputWidth);
        this.weightedAmplitude += amplitude * overlapWidth;
        remainingSourceWidth -= overlapWidth;
        this.remainingOutputWidth -= overlapWidth;
        if (this.remainingOutputWidth < 1e-9) {
          this.writeSample(this.weightedAmplitude / this.sourceSamplesPerOutputSample);
          this.weightedAmplitude = 0;
          this.remainingOutputWidth = this.sourceSamplesPerOutputSample;
        }
      }
      if (!this.active) break;
    }
    // Output channels stay silent; capture must not play the microphone back.
    return this.active;
  }
}
registerProcessor('thoughts-pcm', ThoughtsPCMProcessor);
