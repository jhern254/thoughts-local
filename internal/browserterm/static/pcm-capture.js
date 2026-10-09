/* One recording owns capture, worklet frames, and a dedicated audio socket. */
(() => {
  'use strict';
  const maximumOutboundBytes = 48000;
  const maximumRecordingMilliseconds = 20 * 60 * 1000;
  globalThis.ThoughtsPCMRecording = class {
    constructor(onState) {
      this.onState = onState;
      this.current = undefined;
      this.preparedContext = undefined;
    }
    // AudioContext activation belongs to the trusted F8 event, before the
    // asynchronous server grant. Capturing still requires accepted authority.
    prepare() {
      if (this.current || this.preparedContext || typeof AudioContext === 'undefined') return;
      try {
        // The browser owns the device processing rate (often 44.1/48 kHz).
        // The worklet converts samples to the fixed 16 kHz wire rate rather
        // than depending on device or engine sample-rate choices.
        this.preparedContext = new AudioContext();
        this.preparedContext.resume().catch(() => {});
      } catch {}
    }
    async start(grant) {
      if (this.current) return;
      const recording = {sessionID: grant.sessionID, draftID: grant.draftID, recordingID: grant.recordingID};
      const run = {recording, audioContext: this.preparedContext, stopping: false};
      this.preparedContext = undefined;
      this.current = run;
      run.deadline = setTimeout(() => this.stop(), maximumRecordingMilliseconds);
      if (!navigator.mediaDevices?.getUserMedia || typeof AudioContext === 'undefined' || typeof AudioWorkletNode === 'undefined') {
        this.finish(run, 'unavailable'); return;
      }
      try {
        const stream = await navigator.mediaDevices.getUserMedia({audio: {channelCount: 1, sampleRate: 16000}});
        if (this.current !== run) { stream.getTracks().forEach(track => track.stop()); return; }
        run.stream = stream;
        stream.getTracks().forEach(track => {
          track.onended = () => { if (this.current === run && !run.stopping) this.finish(run, 'unavailable'); };
        });
        if (!run.audioContext) {
          run.audioContext = new AudioContext();
        }
        await run.audioContext.audioWorklet.addModule('/static/pcm-worklet.js');
        if (this.current !== run) return;
        await this.connectAudio(run, grant);
        if (this.current !== run) return;
        run.processor = new AudioWorkletNode(run.audioContext, 'thoughts-pcm', {
          numberOfInputs: 1, numberOfOutputs: 1, outputChannelCount: [1],
          channelCount: 1, channelCountMode: 'explicit', channelInterpretation: 'speakers'
        });
        run.processor.port.onmessage = event => this.sendPCM(run, event.data);
        run.processor.onprocessorerror = () => this.finish(run, 'failed');
        run.processor.port.postMessage({credit: 4});
        run.source = run.audioContext.createMediaStreamSource(stream);
        run.source.connect(run.processor);
        run.processor.connect(run.audioContext.destination);
        await run.audioContext.resume();
        if (this.current === run) this.onState('recording', recording);
      } catch (error) {
        this.finish(run, error?.name === 'NotAllowedError' ? 'denied' : 'unavailable');
      }
    }
    connectAudio(run, grant) {
      return new Promise((resolve, reject) => {
        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const audioSocket = new WebSocket(`${protocol}//${location.host}/voice/audio`);
        run.audioSocket = audioSocket;
        audioSocket.binaryType = 'arraybuffer';
        audioSocket.onopen = () => {
          if (this.current !== run) { audioSocket.close(); return; }
          const handshake = new Uint8Array(36);
          try {
            handshake.set([84, 65, 1, 1]);
            for (let index = 0; index < 32; index++) handshake[4 + index] = parseInt(grant.audioCapability.slice(index * 2, index * 2 + 2), 16);
            audioSocket.send(handshake);
          } catch {
            reject(new Error('Audio unavailable'));
            this.finish(run, 'failed');
          } finally {
            handshake.fill(0);
            audioSocket.onopen = null;
          }
        };
        audioSocket.onmessage = event => {
          if (this.current !== run) return;
          const frame = new Uint8Array(event.data);
          if (frame.length !== 5 || frame[0] !== 84 || frame[1] !== 65 || frame[2] !== 1 || frame[3] !== 3 || frame[4] !== 0) {
            reject(new Error('Audio unavailable'));
            this.finish(run, 'failed'); return;
          }
          run.ready = true;
          resolve();
        };
        audioSocket.onerror = () => {}; // Close provides the fixed status.
        audioSocket.onclose = () => {
          reject(new Error('Audio unavailable'));
          if (this.current === run && !run.stopping) this.finish(run, 'failed');
        };
      });
    }
    sendPCM(run, message) {
      if (!message.pcm) { if (message.status === 'failed') this.finish(run, 'failed'); return; }
      const pcmBytes = new Uint8Array(message.pcm);
      try {
        if (this.current !== run || run.stopping) return;
        if (!run.ready || pcmBytes.length !== 3200 || run.audioSocket.readyState !== WebSocket.OPEN) {
          this.finish(run, 'failed'); return;
        }
        if (run.audioSocket.bufferedAmount + pcmBytes.length > maximumOutboundBytes) { this.finish(run, 'failed'); return; }
        run.audioSocket.send(pcmBytes);
        run.processor.port.postMessage({credit: 1});
      } catch { this.finish(run, 'failed'); }
      finally { pcmBytes.fill(0); }
    }
    release(run) {
      clearTimeout(run.deadline);
      if (run.processor) {
        run.processor.port.postMessage({stop: true});
        run.processor.disconnect();
        // Late transfers are cleared without retaining the old recording.
        run.processor.port.onmessage = event => { if (event.data.pcm) new Uint8Array(event.data.pcm).fill(0); };
        run.processor.onprocessorerror = null;
        run.processor = undefined;
      }
      if (run.source) { run.source.disconnect(); run.source = undefined; }
      if (run.stream) {
        run.stream.getTracks().forEach(track => { track.onended = null; track.stop(); });
        run.stream = undefined;
      }
      if (run.audioSocket) { run.audioSocket.close(); run.audioSocket = undefined; }
      if (run.audioContext) { run.audioContext.close().catch(() => {}); run.audioContext = undefined; }
    }
    finish(run, status) {
      if (this.current !== run) return;
      this.current = undefined;
      run.stopping = true;
      this.release(run);
      this.onState(status, run.recording);
    }
    stop() {
      const run = this.current;
      if (!run || run.stopping) { this.cancel(); return; }
      run.stopping = true;
      this.onState('stop', run.recording);
      this.finish(run, 'stopped');
    }
    cancel(preservePreparedContext = false) {
      const run = this.current;
      this.current = undefined;
      if (run) { run.stopping = true; this.release(run); }
      if (this.preparedContext && !preservePreparedContext) { this.preparedContext.close().catch(() => {}); this.preparedContext = undefined; }
    }
  };
})();
