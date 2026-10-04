/* Temporary microphone capture for one Thoughts draft. No audio is retained. */
(() => {
  'use strict';
  const maximumDuration = 20 * 60 * 1000;
  const maximumChunk = 8 * 1024 * 1024;
  globalThis.ThoughtsMicrophone = class {
    constructor(onState) {
      this.onState = onState;
      this.current = undefined;
    }
    async start() {
      if (this.current) return;
      const run = {stream: undefined, recorder: undefined, timer: undefined, stopping: false};
      this.current = run;
      if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
        this.finish(run, 'unavailable');
        return;
      }
      try {
        const stream = await navigator.mediaDevices.getUserMedia({audio: true});
        if (this.current !== run) { stream.getTracks().forEach(track => track.stop()); return; }
        run.stream = stream;
        const mimeType = ['audio/webm;codecs=opus', 'audio/ogg;codecs=opus', 'audio/mp4'].find(type => MediaRecorder.isTypeSupported(type));
        const recorder = new MediaRecorder(stream, mimeType ? {mimeType, audioBitsPerSecond: 64000} : {});
        run.recorder = recorder;
        run.started = performance.now();
        recorder.ondataavailable = event => {
          if (this.current !== run || run.stopping) return;
          // Consume and discard every chunk. Never accumulate a recording-sized
          // buffer. Delayed browser events can produce unexpectedly large blobs.
          if (event.data.size > maximumChunk) { this.finish(run, 'failed'); return; }
          if (performance.now() - run.started >= maximumDuration) this.stop();
        };
        recorder.onstop = () => {
          if (this.current === run) this.finish(run, 'stopped');
        };
        recorder.onerror = () => this.finish(run, 'failed');
        stream.getTracks().forEach(track => {
          track.onended = () => { if (this.current === run && !run.stopping) this.finish(run, 'unavailable'); };
        });
        recorder.start(1000);
        run.timer = setTimeout(() => this.stop(), maximumDuration);
        this.onState('recording', run.started);
      } catch (error) {
        this.finish(run, error?.name === 'NotAllowedError' ? 'denied' : 'unavailable');
      }
    }
    stop() {
      const run = this.current;
      if (!run || run.stopping) return;
      run.stopping = true;
      clearTimeout(run.timer);
      this.onState('stop');
      // Permission may remain unanswered indefinitely. Invalidate this attempt
      // now; a late stream is released by start without reviving the draft.
      if (!run.recorder) { this.finish(run, 'stopped'); return; }
      try { if (run.recorder.state !== 'inactive') run.recorder.stop(); }
      catch { this.finish(run, 'failed'); return; }
      this.releaseTracks(run);
    }
    releaseTracks(run) {
      if (!run.stream) return;
      run.stream.getTracks().forEach(track => { track.onended = null; track.stop(); });
      run.stream = undefined;
    }
    finish(run, state) {
      if (this.current !== run) return;
      this.cancel();
      this.onState(state);
    }
    cancel() {
      const run = this.current;
      if (!run) return;
      this.current = undefined;
      clearTimeout(run.timer);
      this.releaseTracks(run);
      try { if (run.recorder && run.recorder.state !== 'inactive') run.recorder.stop(); } catch {}
    }
  };
})();
