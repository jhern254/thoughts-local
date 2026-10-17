/* One new-upload editor. Native handles own staged files; this browser owner
 * owns local source scrubbing and the last acknowledged, encoded loop. */
class ThoughtsVideoTrim {
  constructor(visual) {
    this.visual = visual;
    this.generation = 0;
  }
  headers() { return {'X-Visual-Editor': this.editorID}; }
  owns(generation) {
    return generation === this.generation && this.visual.connected && this.visual.state.open && this.visual.state.editorID === this.editorID;
  }
  async request(path, options, signal) {
    const response = await fetch(path, {...options, headers: {...this.headers(), ...options.headers}, signal});
    if (!response.ok) throw new Error('Video operation unavailable.');
    return response;
  }
  async stage(file, operationID) {
    this.close();
    this.visual.releasePreviewVideo();
    const generation = this.generation;
    this.editorID = this.visual.state.editorID;
    const controller = new AbortController();
    this.controller = controller;
    try {
      if (file.size > 128 * 1024 * 1024) throw new Error();
      const response = await this.request('/visual/video/source', {method:'POST', body:file}, controller.signal);
      const source = await response.json();
      if (!this.owns(generation) || this.visual.state.id !== operationID) return;
      this.source = source;
      this.sourceURL = URL.createObjectURL(file);
      const video = this.makeVideo(this.sourceURL);
      this.sourceVideo = video;
      await this.ready(video, controller.signal);
      if (!this.owns(generation)) return;
      // Native dimensions include display rotation and sample-aspect ratio.
      // Source playback is paused; only explicitly prepared loops can play.
      this.visual.candidateKind = 'video';
      this.visual.previewSourceSize = {width:source.width, height:source.height};
      this.showSource(0);
      this.visual.sendAction({action:'staged', id:operationID, kind:'video', durationMS:source.durationMS});
    } catch (error) {
      if (this.owns(generation)) {this.close(); throw error;}
    } finally {
      if (this.controller === controller) this.controller = undefined;
    }
  }
  makeVideo(url) {
    const video = document.createElement('video');
    video.muted = true; video.loop = true; video.playsInline = true;
    video.preload = 'auto'; video.hidden = true; video.src = url;
    return video;
  }
  ready(video, signal) {
    return new Promise((resolve, reject) => {
      const finish = success => {
        clearTimeout(timeout);
        video.removeEventListener('loadeddata', loaded);
        video.removeEventListener('error', failed);
        signal.removeEventListener('abort', failed);
        success ? resolve() : reject(new Error('Video preview unavailable.'));
      };
      const loaded = () => finish(true), failed = () => finish(false);
      const timeout = setTimeout(failed, 10000);
      video.addEventListener('loadeddata', loaded);
      video.addEventListener('error', failed);
      signal.addEventListener('abort', failed, {once:true});
      if (signal.aborted) failed(); else if (video.readyState >= 2) loaded(); else video.load();
    });
  }
  attach(video) {
    const visual = this.visual;
    visual.stopPreviewPlayback();
    if (visual.previewVideo && visual.previewVideo !== video) visual.previewVideo.remove();
    visual.previewVideo = video;
    video.id = 'background-preview-video';
    visual.previewFrameElement.append(video);
    video.hidden = false;
    visual.queuePreview();
  }
  showSource(positionMS) {
    if (!this.sourceVideo) return;
    if (this.visual.previewVideo !== this.sourceVideo) this.attach(this.sourceVideo);
    // Coalesce seek requests: one in flight and one latest target. No playback
    // timer, frame copying or FFmpeg work occurs while moving cut controls.
    this.seekTarget = Math.min(positionMS / 1000, Math.max(0,this.sourceVideo.duration - 0.001));
    this.seekSource();
  }
  seekSource() {
    const video = this.sourceVideo;
    if (!video || video.seeking || this.seekTarget === undefined) return;
    const target = this.seekTarget;
    this.seekTarget = undefined;
    if (Math.abs(video.currentTime-target) < 0.001) return;
    video.addEventListener('seeked', () => {
      if (this.sourceVideo === video) this.seekSource();
    }, {once:true});
    video.currentTime = target;
  }
  sync(state, previous) {
    if (!this.source) return;
    if (state.trimming && state.sourceFrame && (!previous.sourceFrame || state.sourcePositionMS !== previous.sourcePositionMS)) this.showSource(state.sourcePositionMS);
    if (!state.trimming && previous.trimming && this.acceptedVideo && this.visual.previewVideo !== this.acceptedVideo) this.showAccepted(false);
  }
  async prepare(state) {
    const generation = this.generation;
    const operationID = state.id;
    const visual = this.visual;
    this.controller?.abort();
    const controller = new AbortController();
    this.controller = controller;
    let candidate, videoURL, posterURL;
    try {
      const response = await this.request('/visual/video/preview', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({sourceID:this.source.sourceID, trim:state.trim})},controller.signal);
      const prepared = await response.json();
      if (!this.owns(generation)) return;
      if (this.accepted?.previewID !== prepared.previewID) {
        const videoResponse = await this.request(`/visual/video/preview?id=${encodeURIComponent(prepared.previewID)}&part=video`,{},controller.signal);
        videoURL = URL.createObjectURL(await videoResponse.blob());
        const posterResponse = await this.request(`/visual/video/preview?id=${encodeURIComponent(prepared.previewID)}&part=poster`,{},controller.signal);
        posterURL = URL.createObjectURL(await posterResponse.blob());
        candidate = this.makeVideo(videoURL);
        await this.ready(candidate,controller.signal);
        const poster = new Image(); poster.src = posterURL; await poster.decode();
        if (!this.owns(generation) || controller.signal.aborted) return;
        // Acknowledge only after this exact encoded asset can be displayed.
        // Until then native and browser owners both retain the accepted loop.
        await this.request('/visual/video/preview',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({previewID:prepared.previewID})},controller.signal);
        if (!this.owns(generation) || controller.signal.aborted) return;
        visual.stopPreviewPlayback();
        visual.retireVideo(this.acceptedVideo);
        if (this.acceptedVideoURL) URL.revokeObjectURL(this.acceptedVideoURL);
        if (this.acceptedPosterURL) URL.revokeObjectURL(this.acceptedPosterURL);
        this.acceptedVideo = candidate; this.acceptedVideoURL = videoURL; this.acceptedPosterURL = posterURL;
        candidate = undefined; videoURL = undefined; posterURL = undefined;
        this.accepted = prepared;
      }
      this.showAccepted(true);
      visual.sendAction({action:'loopReady',id:operationID,trim:this.accepted.trim,durationMS:this.accepted.durationMS});
    } catch {
      if (this.owns(generation) && !controller.signal.aborted) {
        if (this.acceptedVideo) this.showAccepted(false);
        visual.reply('failed',operationID);
      }
    } finally {
      visual.retireVideo(candidate);
      if (videoURL) URL.revokeObjectURL(videoURL);
      if (posterURL) URL.revokeObjectURL(posterURL);
      if (this.controller === controller) this.controller = undefined;
    }
  }
  showAccepted(play) {
    if (!this.acceptedVideo) return;
    const visual = this.visual;
    this.attach(this.acceptedVideo);
    visual.preview.src = this.acceptedPosterURL;
    // Decoding/acknowledgement makes a loop applicable, independently of motion
    // preferences. Automatic preview must not override a deliberate still view.
    const motionPermitted = !visual.motionQuery?.matches || visual.motionOverride;
    if (play && !visual.state.userPaused && motionPermitted) visual.togglePreviewPlayback();
  }
  async discard(operationID) {
    const editorID = this.editorID;
    this.close();
    const generation=this.generation;
    try { if (editorID) await fetch('/visual/video/source',{method:'DELETE',headers:{'X-Visual-Editor':editorID}}); } catch { /* Closing Visual also retires native files. */ }
    if (!this.owns(generation) || this.visual.state.id!==operationID) return;
    this.visual.discardPreview();
    if (this.visual.activeURL) this.visual.preview.src = this.visual.activeURL;
    this.visual.reply('discarded',operationID);
  }
  applyRequest(preferences) {
    if (!this.accepted) throw new Error('Preview a loop first.');
    return {method:'POST',headers:{...this.headers(),'Content-Type':'application/json'},body:JSON.stringify({previewID:this.accepted.previewID,...preferences})};
  }
  close() {
    this.generation++;
    this.controller?.abort(); this.controller = undefined;
    const visual = this.visual;
    if (visual.previewVideo === this.sourceVideo || visual.previewVideo === this.acceptedVideo) visual.previewVideo = undefined;
    visual.stopPreviewPlayback();
    visual.retireVideo(this.sourceVideo); visual.retireVideo(this.acceptedVideo);
    for (const url of [this.sourceURL,this.acceptedVideoURL,this.acceptedPosterURL]) if (url) URL.revokeObjectURL(url);
    this.source = undefined; this.accepted = undefined;
    this.sourceVideo = undefined; this.acceptedVideo = undefined;
    this.sourceURL = undefined; this.acceptedVideoURL = undefined; this.acceptedPosterURL = undefined;
    this.seekTarget = undefined;
  }
}
