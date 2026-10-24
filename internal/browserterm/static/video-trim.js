/* One new-upload editor. Native handles own staged files; this browser owner
 * owns local source scrubbing and the last acknowledged, encoded loop. */
class ThoughtsVideoTrim {
  constructor(visual) {
    this.visual = visual;
    this.generation = 0;
    this.sourceStartSeconds = 0;
  }
  headers() { return {'X-Visual-Editor': this.editorID}; }
  owns(generation) {
    return generation === this.generation && this.visual.connected && this.visual.state.open && this.visual.state.editorID === this.editorID;
  }
  async request(path, options, signal) {
    const response = await fetch(path, {...options, headers: {...this.headers(), ...options.headers}, signal});
    if (!response.ok) {
      const error=new Error('Video operation unavailable.');
      if(response.status===507) error.videoAction='sourceSpace';
      if(response.status===408) error.videoAction='sourceTimeout';
      throw error;
    }
    return response;
  }
  async selectSource(file, operationID) {
    this.close();
    this.visual.releasePreviewVideo();
    const generation = this.generation;
    this.editorID = this.visual.state.editorID;
    const controller = new AbortController();
    this.controller = controller;
    try {
      if (file.size > this.visual.state.maxVideoSourceBytes) throw new Error('Video source exceeds the limit.');
      this.localFile = file;
      this.sourceURL = URL.createObjectURL(file);
      const video = this.makeVideo(this.sourceURL, 'metadata');
      this.sourceVideo = video;
      await this.ready(video, controller.signal);
      if (!this.owns(generation) || this.visual.state.id !== operationID) return;
      // Some browsers retain a file's timestamp offset; others start at zero.
      // Cut controls always count from the first decoded source frame.
      this.sourceStartSeconds = video.currentTime;
      const durationMS = Math.floor(video.duration * 1000);
      if (!Number.isSafeInteger(durationMS) || durationMS < 1000 || durationMS > this.visual.state.maxVideoSourceDurationMS || !video.videoWidth || !video.videoHeight) throw new Error('Unsupported source.');
      // Browser metadata is provisional. Only native staging can validate the
      // container, streams, duration and supported display interpretation.
      this.visual.candidateKind = 'video';
      this.visual.previewSourceSize = {width:video.videoWidth, height:video.videoHeight};
      this.showSource(0);
      this.visual.sendAction({action:'sourceSelected', id:operationID, kind:'video', durationMS});
    } catch (error) {
      if (this.owns(generation)) {this.close(); throw error;}
    } finally {
      if (this.controller === controller) this.controller = undefined;
    }
  }
  setStage(label, operationID) {
    if (this.visual.state.id !== operationID) return;
    const status = document.getElementById('video-preparation-label');
    if (status) status.textContent = label;
  }
  uploadSource(signal, operationID) {
    // XHR supplies an upload-complete event without buffering the File in JS.
    // Completion only changes status: the response still owns native validation.
    return new Promise((resolve,reject) => {
      const upload = new XMLHttpRequest();
      const finish = (error,source) => {
        signal.removeEventListener('abort',abort);
        upload.onload=upload.onerror=upload.onabort=upload.ontimeout=null;
        upload.upload.onload=null;
        error ? reject(error) : resolve(source);
      };
      const abort = () => upload.abort();
      upload.open('POST','/visual/video/source');
      upload.setRequestHeader('X-Visual-Editor',this.editorID);
      upload.responseType='json';
      upload.timeout=22*60*1000;
      upload.upload.onload=()=>{if (!signal.aborted) this.setStage('Validating source…',operationID);};
      upload.onload=()=>{
        if (upload.status===200 && upload.response?.sourceID) {finish(null,upload.response);return;}
        const error=new Error('Video source unavailable.');
        error.videoAction=upload.status===507 ? 'sourceSpace' : upload.status===408 ? 'sourceTimeout' : [400,413].includes(upload.status) ? 'sourceUnsupported' : 'failed';
        finish(error);
      };
      upload.onerror=upload.onabort=()=>finish(new Error('Video transfer unavailable.'));
      upload.ontimeout=()=>{const error=new Error('Video transfer timed out.');error.videoAction='sourceTimeout';finish(error);};
      signal.addEventListener('abort',abort,{once:true});
      if (signal.aborted) {finish(new Error('Video transfer cancelled.'));return;}
      upload.send(this.localFile);
    });
  }
  makeVideo(url, preload = 'auto') {
    const video = document.createElement('video');
    video.muted = true; video.loop = true; video.playsInline = true;
    video.preload = preload; video.hidden = true; video.src = url;
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
    visual.sourcePreviewStatus.hidden = true;
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
    if (this.source) this.visual.previewSourceSize = {width:this.source.width,height:this.source.height};
    if (this.visual.previewVideo !== this.sourceVideo) this.attach(this.sourceVideo);
    // Coalesce seek requests: one in flight and one latest target. No playback
    // timer, frame copying or FFmpeg work occurs while moving cut controls.
    const targetSeconds = this.sourceStartSeconds + positionMS / 1000;
    // Chromium can retain a nonzero origin but clamp seeks to the unshifted
    // duration. Native Preview loop can still prepare those later source cuts.
    const unavailable = this.sourceStartSeconds > 0 && targetSeconds >= this.sourceVideo.duration;
    this.visual.sourcePreviewStatus.hidden = !unavailable;
    if (unavailable) { this.seekTarget = undefined; return; }
    this.seekTarget = Math.min(targetSeconds, this.sourceVideo.duration - 0.001);
    this.seekSource();
  }
  seekSource() {
    const video = this.sourceVideo;
    if (!video || video.seeking || this.seekTarget === undefined) return;
    const target = this.seekTarget;
    this.seekTarget = undefined;
    if (Math.abs(video.currentTime-target) < 0.001) { video.hidden = false; return; }
    video.hidden = true;
    video.addEventListener('seeked', () => {
      if (this.sourceVideo === video) {
        if (this.seekTarget === undefined) video.hidden = false;
        this.seekSource();
      }
    }, {once:true});
    video.currentTime = target;
  }
  sync(state, previous) {
    if (!this.localFile) return;
    if (state.trimming && state.sourceFrame && (!previous.sourceFrame || state.trim.startMS !== previous.trim?.startMS)) this.showSource(state.trim.startMS);
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
      if (!this.source) {
        this.setStage('Transferring selected source…', operationID);
        const staged = await this.uploadSource(controller.signal, operationID);
        if (!this.owns(generation) || controller.signal.aborted || visual.state.id !== operationID) return;
        this.source = staged;
        visual.sendAction({action:'sourceVerified',id:operationID,durationMS:this.source.durationMS});
      }
      const {startMS,endMS}=state.trim;
      if (startMS < 0 || endMS > this.source.durationMS || endMS-startMS < 1000 || endMS-startMS > 30000) {
        visual.sendAction({action:'sourceBounds',id:operationID});
        return;
      }
      this.setStage(`Preparing ${(endMS-startMS)/1000}-second loop…`, operationID);
      const response = await this.request('/visual/video/preview', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({sourceID:this.source.sourceID, trim:state.trim,videoQuality:state.videoQuality})},controller.signal);
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
      visual.sendAction({action:'loopReady',id:operationID,trim:this.accepted.trim,durationMS:this.accepted.durationMS,videoQuality:this.accepted.videoQuality,width:this.accepted.width,height:this.accepted.height,frameRate:this.accepted.frameRate});
    } catch (error) {
      if (this.owns(generation) && !controller.signal.aborted && visual.state.id === operationID) {
        if (this.acceptedVideo) this.showAccepted(false);
        visual.reply(error.videoAction || 'failed',operationID);
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
    if (this.accepted) visual.previewSourceSize = {width:this.accepted.width,height:this.accepted.height};
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
    const {videoQuality, ...displayPreferences} = preferences;
    return {method:'POST',headers:{...this.headers(),'Content-Type':'application/json'},body:JSON.stringify({previewID:this.accepted.previewID,...displayPreferences})};
  }
  close() {
    this.generation++;
    this.controller?.abort(); this.controller = undefined;
    const visual = this.visual;
    if (visual.previewVideo === this.sourceVideo || visual.previewVideo === this.acceptedVideo) visual.previewVideo = undefined;
    visual.stopPreviewPlayback();
    visual.retireVideo(this.sourceVideo); visual.retireVideo(this.acceptedVideo);
    for (const url of [this.sourceURL,this.acceptedVideoURL,this.acceptedPosterURL]) if (url) URL.revokeObjectURL(url);
    this.source = undefined; this.localFile = undefined; this.accepted = undefined;
    this.sourceVideo = undefined; this.acceptedVideo = undefined;
    this.sourceURL = undefined; this.acceptedVideoURL = undefined; this.acceptedPosterURL = undefined;
    this.seekTarget = undefined;
    this.sourceStartSeconds = 0;
    this.visual.sourcePreviewStatus.hidden = true;
  }
}
