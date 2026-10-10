// Generated Web Audio and Firefox synthetic-device tracks exercise the real
// AudioWorklet without a physical device or audio file. Permission replies stay explicit.
export async function fakeMicrophone(page, amplitude = 0, nativeSyntheticDevice = false) {
  await page.addInitScript(({amplitude, nativeSyntheticDevice}) => {
    const nativeGetUserMedia = navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);
    window.voiceProbe = {requests: 0, tracksStopped: 0, recorders: [], permissions: []};
    Object.defineProperty(navigator, 'mediaDevices', {value: {getUserMedia: () => {
      window.voiceProbe.requests++;
      return new Promise((resolve, reject) => window.voiceProbe.permissions.push({resolve, reject}));
    }}});
    window.voiceProbe.grant = async index => {
      if (nativeSyntheticDevice) {
        const stream = await nativeGetUserMedia({audio: true});
        for (const track of stream.getTracks()) {
          const stopTrack = track.stop.bind(track);
          let stopped = false;
          track.stop = () => { if (!stopped) { stopped = true; window.voiceProbe.tracksStopped++; stopTrack(); } };
        }
        window.voiceProbe.permissions[index].resolve(stream);
        return;
      }
      const audioContext = new AudioContext();
      const destination = audioContext.createMediaStreamDestination();
      const source = audioContext.createBufferSource();
      source.buffer = audioContext.createBuffer(1, 1600, 16000);
      const audioSamples = source.buffer.getChannelData(0);
      for (let sampleIndex = 0; sampleIndex < audioSamples.length; sampleIndex++) {
        audioSamples[sampleIndex] = amplitude * Math.sin(2 * Math.PI * 1000 * sampleIndex / 16000);
      }
      source.loop = true;
      source.connect(destination);
      const silentOutput = audioContext.createGain();
      silentOutput.gain.value = 0;
      source.connect(silentOutput);
      silentOutput.connect(audioContext.destination);
      window.voiceProbe.generator = {source, destination, silentOutput};
      source.start();
      await audioContext.resume();
      for (const track of destination.stream.getTracks()) {
        const stopTrack = track.stop.bind(track);
        let stopped = false;
        track.stop = () => {
          if (stopped) return;
          stopped = true;
          window.voiceProbe.tracksStopped++;
          stopTrack(); source.stop(); audioContext.close();
        };
      }
      window.voiceProbe.permissions[index].resolve(destination.stream);
    };
  }, {amplitude, nativeSyntheticDevice});
}
