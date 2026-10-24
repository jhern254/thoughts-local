/* Bubble Tea owns controls; the browser owns the picker, background media and HTTP. */
// Bound the preview canvas to roughly 4 MiB of RGBA pixels without collapsing
// thin panoramas to a single row or column merely to satisfy an axis limit.
const backgroundPreviewMaxPixels = 1200 * 800;

class ThoughtsVisual {
  constructor(terminal, sendAction) {
    this.terminal = terminal;
    this.sendAction = sendAction;
    this.image = document.getElementById('background-image');
    this.surface = document.getElementById('background-surface');
    this.previewFrameElement = document.getElementById('background-preview-frame');
    this.overlay = document.getElementById('background-overlay');
    this.file = document.getElementById('background-file');
    this.preview = document.getElementById('background-preview');
    this.previewContainer = document.getElementById('background-preview-wrap');
    this.saved = {background: false, darkness: 42, framing: defaultBackgroundFraming(), speedPercent: 100, userPaused: false, videoQuality:'efficient', kind: 'image'};
    this.connected = false;
    this.documentVisible = !document.hidden;
    this.motionQuery = matchMedia('(prefers-reduced-motion: reduce)');
    this.playbackStatus = document.getElementById('video-status');
    this.sourcePreviewStatus = document.getElementById('source-preview-status');
    this.preparationStatus = document.getElementById('video-preparation');
    this.preparationWave = new ThoughtsPreparationWave(document.getElementById('video-preparation-wave'));
    this.motionQuery.addEventListener('change', () => {
      this.motionOverride = false;
      if (this.motionQuery.matches) this.stopPreviewPlayback();
      this.updatePlayback(); this.reportPlayback();
      this.updatePreparationStatus();
    });
    document.addEventListener('visibilitychange', () => { this.documentVisible = !document.hidden; this.updatePlayback(); this.updatePreparationStatus(); if (document.hidden) this.stopPreviewPlayback(); });
    window.addEventListener('pagehide', () => this.preparationWave.setPlayback(false, false, false));
    this.state = {open: false};
    this.videoTrim = new ThoughtsVideoTrim(this);
    this.previewGeneration = 0;
    this.connectionGeneration = 0;
    // A connection owns its async work; activation generations also order
    // competing image/settings publications within that connection.
    this.activationGeneration = 0;
    this.restoreGeneration = 0;
    this.ready = this.restore();
    document.addEventListener('keydown', event => {
      if ((event.ctrlKey || event.metaKey) && event.key === ',') {
        event.preventDefault();
        event.stopImmediatePropagation();
        this.sendAction({action: 'open'});
      }
    }, true);
    this.preview.addEventListener('load', () => this.queuePreview());
    this.previewFrameElement.addEventListener('pointerdown', event => this.startFramingDrag(event));
    this.previewFrameElement.addEventListener('pointermove', event => this.moveFramingDrag(event));
    this.previewFrameElement.addEventListener('pointerup', event => this.endFramingDrag(event));
    this.previewFrameElement.addEventListener('pointercancel', () => this.cancelFramingDrag());
    this.previewFrameElement.addEventListener('lostpointercapture', () => this.cancelFramingDrag());
    window.addEventListener('resize', () => { this.cancelFramingDrag(); this.renderBackground(); this.queuePreview(); });
    this.file.addEventListener('change', () => this.previewFile());
    this.file.addEventListener('cancel', () => this.reply('cancelled', this.state.id));
    this.terminal.onRender(() => {
      if (this.refreshAfterClose) {
        this.refreshAfterClose = false;
        // Closing metadata arrives before the returning TUI screen. Repaint
        // every row after that render to retire pixels behind the image overlay.
        this.terminal.refresh(0, this.terminal.rows - 1);
      }
      this.queuePreview();
    });
    new ResizeObserver(() => { this.previewContainer.hidden = true; this.queuePreview(); }).observe(this.terminal.element);
  }
  isOpen() { return this.state.open; }
  restoreConnection() {
    this.connected = true;
    this.updatePlayback();
    // A busy-session retry may retire the page's initial restore. The accepted
    // session needs its own restore, without refetching on every normal open.
    if (this.restoreGeneration !== this.connectionGeneration) {
      this.restoreGeneration = this.connectionGeneration;
      this.ready = this.restore();
    }
  }
  reply(action, operationID, connectionGeneration = this.connectionGeneration) {
    if (connectionGeneration === this.connectionGeneration && this.state.open && this.state.id === operationID) {
      this.sendAction({action, id: operationID, videoQuality: this.saved.videoQuality || 'efficient', darkness: this.saved.darkness, background: this.saved.background, framing: this.saved.framing, kind: this.candidateKind || this.saved.kind, speedPercent: this.saved.speedPercent, userPaused: this.saved.userPaused, playbackBlocked: this.playbackFailed || (this.motionQuery?.matches && !this.motionOverride)});
    }
  }
  async receive(state) {
    const previousState = this.state;
    this.state = state;
    this.updatePreparationStatus();
    if (!state.open || !state.editing || state.id !== previousState.id) this.cancelFramingDrag();
    this.refreshAfterClose = previousState.open && !state.open;
    if (JSON.stringify(state.preview) !== JSON.stringify(previousState.preview)) this.previewContainer.hidden = true;
    if (!state.open) {
      if (previousState.request === 'apply') this.saveController?.abort();
      this.discardPreview();
      this.updateDarkness(this.saved.darkness);
      if (previousState.open) this.terminal.focus();
      return;
    }
    this.updateDarkness(state.request === 'load' ? this.saved.darkness : state.darkness);
    this.videoTrim?.sync(state,previousState);
    this.queuePreview();
    this.updatePlayback();
    if (this.previewVideo) this.previewVideo.playbackRate = this.playbackSpeed();
    if (state.id === previousState.id && state.request === previousState.request) return;
    const operationID = state.id;
    const connectionGeneration = this.connectionGeneration;
    switch (state.request) {
      case 'load':
        await this.ready;
        if (connectionGeneration !== this.connectionGeneration || !this.state.open || this.state.id !== operationID) return;
        if (this.activeURL) this.preview.src = this.activeURL;
        this.reply(this.loadFailed ? 'failed' : 'loaded', operationID);
        this.terminal.focus();
        break;
      case 'choose':
        // Keep the native picker attached to the existing document. No terminal
        // replacement, reconnect or microphone operation is involved.
        this.file.click();
        break;
      case 'prepareLoop':
        await this.videoTrim.prepare(state); break;
      case 'stopPreview':
        await this.videoTrim.stopRendering(operationID); break;
      case 'discardVideo':
        await this.videoTrim.discard(operationID); break;
      case 'play':
        this.motionOverride = !state.userPaused; this.playbackFailed = false; this.updatePlayback();
        this.reply('played', operationID); break;
      case 'preview':
        await this.togglePreviewPlayback(); this.reply('previewed', operationID, connectionGeneration); break;
      case 'apply': case 'remove':
        await this.save(state.request === 'remove', operationID, state.darkness, state.framing, connectionGeneration);
        break;
    }
  }
  updatePreparationStatus() {
    // The current request owns the indicator through encoding, validation and
    // preview acknowledgement. Retired async completions cannot hide a new one.
    const active = !!(this.connected && this.state.open && this.state.trimming && this.state.request === 'prepareLoop');
    this.preparationStatus.hidden = !active;
    this.preparationWave.setPlayback(active, this.documentVisible, this.motionQuery.matches);
    if (active) this.positionPreparation();
  }
  positionPreparation() {
    if (this.preparationStatus.hidden || !this.state.preparation) return;
    const screen = this.terminal.element.querySelector('.xterm-screen').getBoundingClientRect();
    const cellWidth = screen.width / this.terminal.cols;
    const cellHeight = screen.height / this.terminal.rows;
    const reserved = this.state.preparation;
    const width = reserved.Width * cellWidth;
    Object.assign(this.preparationStatus.style, {
      left: `${screen.left + reserved.X * cellWidth}px`, width: `${width}px`,
      top: `${screen.top + reserved.Y * cellHeight}px`
    });
    const wave = document.getElementById('video-preparation-wave');
    const layer = this.preparationWave.trace;
    // Measure only on layout/terminal paints, never in the animation loop.
    if (layer.offsetWidth && layer.offsetHeight) {
      wave.style.setProperty('--wave-width', `${width}px`);
      wave.style.setProperty('--wave-tail', `${width * 64 / 156}px`);
      wave.style.setProperty('--wave-dot', `${width / 156}px`);
      wave.style.setProperty('--wave-scale-x', width / layer.offsetWidth);
      wave.style.setProperty('--wave-scale-y', wave.clientHeight / layer.offsetHeight);
    }
  }
  disconnect() {
    this.connected = false;
    this.motionOverride = false;
    this.saveController?.abort();
    this.updatePlayback();
    this.connectionGeneration++;
    this.state = {open: false};
    this.updatePreparationStatus();
    this.refreshAfterClose = false;
    this.discardPreview();
    this.updateDarkness(this.saved.darkness);
  }
  renderBackground() {
    if (!this.saved.background || !this.image.naturalWidth) return;
    this.placeImage(this.image, window.innerWidth, window.innerHeight, this.saved.framing);
    if (this.video) this.placeImage(this.video, window.innerWidth, window.innerHeight, this.saved.framing);
  }
  placeImage(image, width, height, framing, sourceDimensions) {
    const mediaWidth = image.naturalWidth || image.videoWidth;
    const mediaHeight = image.naturalHeight || image.videoHeight;
    if (!mediaWidth || !mediaHeight) return;
    const geometry = backgroundGeometry(
      sourceDimensions?.width || mediaWidth,
      sourceDimensions?.height || mediaHeight,
      width, height, safeBackgroundFraming(framing)
    );
    for (const [name, value] of Object.entries(geometry)) image.style[name] = `${value}px`;
  }
  startFramingDrag(event) {
    if (!this.state.open || !this.state.editing || !this.preview.naturalWidth || event.button !== 0) return;
    event.preventDefault();
    this.terminal.focus();
    const bounds = this.previewFrameElement.getBoundingClientRect();
    const framing = {...this.state.framing};
    const geometry = backgroundGeometry(
      this.previewSourceSize?.width || this.preview.naturalWidth,
      this.previewSourceSize?.height || this.preview.naturalHeight,
      bounds.width, bounds.height, framing
    );
    this.drag = {
      pointerID: event.pointerId,
      operationID: this.state.id,
      x: event.clientX,
      y: event.clientY,
      framing,
      overflowX: bounds.width - geometry.width,
      overflowY: bounds.height - geometry.height
    };
    this.previewFrameElement.setPointerCapture(event.pointerId);
  }
  moveFramingDrag(event) {
    const drag = this.drag;
    if (!drag || drag.pointerID !== event.pointerId) return;
    const position = (start, delta, overflow) => {
      // An axis with no overflow cannot pan. Ignore subpixel rounding noise.
      if (overflow >= -0.01) return start;
      return Math.round(Math.max(0, Math.min(10000, start + delta / overflow * 10000)));
    };
    this.dragFraming = {
      ...drag.framing,
      positionX: position(drag.framing.positionX, event.clientX - drag.x, drag.overflowX),
      positionY: position(drag.framing.positionY, event.clientY - drag.y, drag.overflowY)
    };
    if (!this.dragFrame) this.dragFrame = requestAnimationFrame(() => this.sendFramingDrag());
  }
  sendFramingDrag() {
    this.dragFrame = undefined;
    if (this.drag && this.dragFraming && this.state.open && this.state.editing && this.state.id === this.drag.operationID) {
      this.sendAction({action: 'reframe', id: this.drag.operationID, framing: this.dragFraming});
    }
  }
  endFramingDrag(event) {
    if (!this.drag || this.drag.pointerID !== event.pointerId) return;
    this.moveFramingDrag(event);
    cancelAnimationFrame(this.dragFrame);
    this.sendFramingDrag();
    this.cancelFramingDrag();
  }
  cancelFramingDrag() {
    cancelAnimationFrame(this.dragFrame);
    this.dragFrame = undefined;
    const pointerID = this.drag?.pointerID;
    this.drag = undefined;
    this.dragFraming = undefined;
    if (pointerID !== undefined && this.previewFrameElement.hasPointerCapture(pointerID)) this.previewFrameElement.releasePointerCapture(pointerID);
  }
  updateDarkness(value) {
    value = Number.isInteger(value) ? Math.max(0, Math.min(95, value)) : 42;
    this.previewContainer.style.setProperty('--darkness', value / 100);
    this.overlay.style.backgroundColor = `rgb(0 0 0 / ${value}%)`;
  }
  queuePreview() {
    if (!this.state.open) { this.previewContainer.hidden = true; return; }
    cancelAnimationFrame(this.previewFrame);
    this.previewFrame = requestAnimationFrame(() => this.positionPreview());
  }
  positionPreview() {
    this.positionPreparation();
    this.previewContainer.hidden = true;
    const previewRect = this.state.preview;
    const awaitingSourceFrame = this.videoTrim?.showingSource && this.videoTrim.sourcePreviewMode === 'native';
    if (!this.state.open || !previewRect || !previewRect.Height || (!this.preview.getAttribute('src') && !this.previewVideo?.videoWidth && !awaitingSourceFrame)) return;
    if (this.state.request === 'load' || this.terminal.cols < previewRect.X + previewRect.Width + 1 || this.terminal.rows < previewRect.Y + previewRect.Height + 1) return;
    // Metadata can arrive before Bubble Tea's next paint. Verify the reserved
    // cells in xterm's public buffer before placing pixels over them. Geometry
    // comes from the TUI; buffer reads only guard against a stale/partial paint.
    const rowText = row => this.terminal.buffer.active.getLine(row)?.translateToString(false) || '';
    if (rowText(previewRect.Y - 1).slice(previewRect.X - 1, previewRect.X + previewRect.Width + 1) !== '╭' + '─'.repeat(previewRect.Width) + '╮') return;
    if (rowText(previewRect.Y + previewRect.Height).slice(previewRect.X - 1, previewRect.X + previewRect.Width + 1) !== '╰' + '─'.repeat(previewRect.Width) + '╯') return;
    for (let row = previewRect.Y; row < previewRect.Y + previewRect.Height; row++) {
      if (rowText(row).slice(previewRect.X - 1, previewRect.X + previewRect.Width + 1) !== '│' + ' '.repeat(previewRect.Width) + '│') return;
    }
    const screenBounds = this.terminal.element.querySelector('.xterm-screen').getBoundingClientRect();
    const cellWidth = screenBounds.width / this.terminal.cols;
    const cellHeight = screenBounds.height / this.terminal.rows;
    Object.assign(this.previewContainer.style, {
      left: `${screenBounds.left + previewRect.X * cellWidth}px`, top: `${screenBounds.top + previewRect.Y * cellHeight}px`,
      width: `${previewRect.Width * cellWidth}px`, height: `${previewRect.Height * cellHeight}px`
    });
    const availableWidth = previewRect.Width * cellWidth;
    const availableHeight = previewRect.Height * cellHeight;
    // Scene selection shows the complete movie; wallpaper framing still previews
    // the browser viewport crop outside the trimmer.
    const aspect = this.state.trimming && this.previewSourceSize
      ? this.previewSourceSize : {width: window.innerWidth, height: window.innerHeight};
    const scale = Math.min(availableWidth / aspect.width, availableHeight / aspect.height);
    const width = aspect.width * scale;
    const height = aspect.height * scale;
    Object.assign(this.previewFrameElement.style, {
      left: `${(availableWidth-width)/2}px`, top: `${(availableHeight-height)/2}px`, width: `${width}px`, height: `${height}px`
    });
    this.previewFrameElement.dataset.editing = String(!!this.state.editing);
    const framing = this.state.trimming ? {...defaultBackgroundFraming(), fit: 'fit'} : this.state.framing;
    this.placeImage(this.preview, width, height, framing, this.previewSourceSize);
    if (this.previewVideo) this.placeImage(this.previewVideo, width, height, framing, this.previewSourceSize);
    this.previewContainer.hidden = false;
  }
  discardPreview() {
    this.videoTrim?.close();
    this.releasePreviewVideo();
    this.candidateKind = undefined;
    this.cancelFramingDrag();
    cancelAnimationFrame(this.previewFrame);
    this.previewContainer.hidden = true;
    this.previewGeneration++;
    if (this.previewURL) URL.revokeObjectURL(this.previewURL);
    this.previewURL = undefined;
    this.previewSourceSize = undefined;
    this.preview.removeAttribute('src');
    this.file.value = '';
    this.updatePlayback();
  }
  async previewFile() {
    const operationID = this.state.id;
    if (!this.state.open || this.state.request !== 'choose') { this.file.value = ''; return; }
    const previewGeneration = ++this.previewGeneration;
    const file = this.file.files[0];
    if (!file) {this.reply('cancelled', operationID); return;}
    try {
      const signature = new Uint8Array(await file.slice(0, 12).arrayBuffer());
      if (String.fromCharCode(...signature.slice(4, 8)) === 'ftyp' ||
          (signature[0] === 0x1a && signature[1] === 0x45 && signature[2] === 0xdf && signature[3] === 0xa3)) {
        await this.videoTrim.selectSource(file, operationID); return;
      }
      if (file.size > 16 * 1024 * 1024) throw new Error();
      if (this.videoTrim?.localFile) {
        const editorID = this.videoTrim.editorID;
        this.videoTrim.close();
        await fetch('/visual/video/source',{method:'DELETE',headers:{'X-Visual-Editor':editorID}});
      }
      this.releasePreviewVideo(); this.candidateKind = 'image';
      const bytes = new Uint8Array(await file.slice(0, 8).arrayBuffer());
      const png = bytes.join(',') === '137,80,78,71,13,10,26,10';
      const jpeg = bytes[0] === 255 && bytes[1] === 216 && bytes[2] === 255;
      if (!png && !jpeg) throw new Error();
      const bitmap = await createImageBitmap(file);
      try {
        if (bitmap.width > 8192 || bitmap.height > 8192 || bitmap.width * bitmap.height > 24000000) throw new Error();
        // Static preview also prevents an animated PNG from animating before
        // the authoritative native importer rejects it.
        const canvas = document.createElement('canvas');
        // createImageBitmap dimensions already reflect display orientation.
        // Predict native storage first, then make a smaller preview thumbnail;
        // the thumbnail pixel cap must not redefine the saved framing geometry.
        const stored = backgroundImportDimensions(bitmap.width, bitmap.height);
        const scale = Math.min(1, Math.sqrt(backgroundPreviewMaxPixels / (stored.width * stored.height)));
        canvas.width = Math.max(1, Math.round(stored.width * scale));
        canvas.height = Math.max(1, Math.round(stored.height * scale));
        canvas.getContext('2d').drawImage(bitmap, 0, 0, canvas.width, canvas.height);
        const blob = await new Promise(resolve => canvas.toBlob(resolve));
        if (!blob || previewGeneration !== this.previewGeneration || this.state.id !== operationID) return;
        if (this.previewURL) URL.revokeObjectURL(this.previewURL);
        this.previewURL = URL.createObjectURL(blob);
        // Thumbnail rounding must not change the crop of very thin images.
        this.previewSourceSize = stored;
        this.preview.src = this.previewURL;
        this.queuePreview();
        this.reply('selected', operationID);
      } finally { bitmap.close(); }
    } catch (error) {
      if (previewGeneration !== this.previewGeneration || this.state.id !== operationID) return;
      this.discardPreview();
      if (this.activeURL) this.preview.src = this.activeURL;
      this.reply(error.videoAction || 'failed', operationID);
    } finally { this.terminal.focus(); }
  }
  async activate(settings, connectionGeneration = this.connectionGeneration) {
    if (connectionGeneration !== this.connectionGeneration) return false;
    // Advance even for removal/settings-only saves, retiring any older decode.
    const activationGeneration = ++this.activationGeneration;
    settings = {...settings};
    if (!Number.isInteger(settings.darkness) || settings.darkness < 0 || settings.darkness > 95) settings.darkness = 42;
    settings.framing = safeBackgroundFraming(settings.framing);
    // Settings-only saves reuse the already decoded asset; identity changes
    // still fetch the authoritative selected image through the existing route.
    if (settings.background && (!this.saved.background || !settings.backgroundID || settings.backgroundID !== this.saved.backgroundID)) {
      const response = await fetch(settings.kind === 'video' ? '/visual/poster' : '/visual/background', {cache: 'no-store'});
      if (!response.ok) throw new Error();
      const url = URL.createObjectURL(await response.blob());
      const candidate = new Image();
      candidate.src = url;
      try { await candidate.decode(); } catch (error) { URL.revokeObjectURL(url); throw error; }
      // Fetch/decode can finish after disconnect or a newer save. Retire its
      // candidate URL before it can replace the currently selected background.
      if (connectionGeneration !== this.connectionGeneration || activationGeneration !== this.activationGeneration) {
        URL.revokeObjectURL(url);
        return false;
      }
      // Publish the decoded element with viewport geometry already prepared.
      // Assigning its URL to another image reopens a load/size gap as the
      // terminal becomes transparent, especially on the first odd-sized upload.
      candidate.id = this.image.id;
      candidate.alt = this.image.alt;
      this.placeImage(candidate, window.innerWidth, window.innerHeight, settings.framing);
      this.retireVideo(this.video);
      this.video = undefined;
      this.image.replaceWith(candidate);
      this.image = candidate;
      if (this.activeURL) URL.revokeObjectURL(this.activeURL);
      this.activeURL = url;
      if (settings.kind === 'video') this.installBackgroundVideo(settings);
    } else if (!settings.background) {
      this.retireVideo(this.video); this.video = undefined;
      if (this.activeURL) URL.revokeObjectURL(this.activeURL);
      this.activeURL = undefined;
      this.image.removeAttribute('src');
    }
    this.saved = settings;
    this.surface.hidden = !settings.background;
    this.renderBackground();
    this.image.hidden = !settings.background;
    this.overlay.hidden = !settings.background;
    this.terminal.options.theme = {...this.terminal.options.theme, background: settings.background ? '#00000000' : '#171717'};
    this.updateDarkness(settings.darkness);
    this.updatePlayback();
    return true;
  }
  async restore() {
    const connectionGeneration = this.connectionGeneration;
    // Fence the metadata request too: a late restore must not undo a newer save
    // before activate gets a chance to fence its own fetch/decode work.
    let activationGeneration = ++this.activationGeneration;
    try {
      const response = await fetch('/visual', {cache: 'no-store'});
      if (!response.ok) throw new Error();
      const settings = await response.json();
      if (activationGeneration !== this.activationGeneration) return;
      const activation = this.activate(settings, connectionGeneration);
      // activate increments the generation synchronously before its first await;
      // follow that ownership so only this restore can report its load failure.
      activationGeneration = this.activationGeneration;
      if (!await activation) return;
      this.loadFailed = false;
    } catch {
      if (connectionGeneration === this.connectionGeneration && activationGeneration === this.activationGeneration) this.loadFailed = true;
    }
  }
  async save(removeBackground, operationID, darkness, framing, connectionGeneration) {
    this.saveController?.abort();
    const controller = new AbortController();
    this.saveController = controller;
    try {
      this.stopPreviewPlayback();
      const preferences = {darkness, framing, videoQuality:this.state.videoQuality || 'efficient', speedPercent: this.state.speedPercent || 100, userPaused: !!this.state.userPaused};
      let requestURL = '/visual/settings';
      let requestOptions = {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(preferences)};
      if (removeBackground) {
        requestURL = '/visual/background'; requestOptions = {method: 'DELETE'};
      } else if (this.file.files[0] && this.candidateKind === 'video') {
        requestURL = '/visual/video';
        requestOptions = this.videoTrim.applyRequest(preferences);
      } else if (this.file.files[0]) {
        const form = new FormData();
        form.append('image', this.file.files[0]); form.append('darkness', darkness);
        form.append('framing', JSON.stringify(framing)); form.append('videoQuality',preferences.videoQuality);
        requestURL = '/visual/background'; requestOptions = {method: 'POST', body: form};
      }
      const response = await fetch(requestURL, {...requestOptions, signal: controller.signal});
      if (!response.ok) throw new Error();
      const settings = await response.json();
      if (!await this.activate(settings, connectionGeneration)) return;
      if (settings.cleanupWarning) {
        this.discardPreview();
        if (this.activeURL) this.preview.src = this.activeURL;
      }
      this.reply(settings.cleanupWarning ? 'warning' : 'saved', operationID, connectionGeneration);
    } catch {
      // Reconcile an ambiguous lost response with authoritative native state.
      if (connectionGeneration !== this.connectionGeneration) return;
      await this.restore();
      this.reply('failed', operationID, connectionGeneration);
    } finally {
      if (this.saveController === controller) this.saveController = undefined;
    }
  }
  reportPlayback() {
    if (this.state.open && this.sendAction) this.sendAction({action: 'playback', id: this.state.id,
      playbackBlocked: !!(this.playbackFailed || (this.motionQuery?.matches && !this.motionOverride))});
  }
  playbackSpeed() {
    // Until the load reply arrives, the Options defaults are not user preferences.
    const preferences = this.state.open && this.state.request !== 'load' ? this.state : this.saved;
    const percent = preferences.speedPercent;
    return Number.isInteger(percent) ? Math.max(25, Math.min(100, percent)) / 100 : 1;
  }
  shouldPlayBackground() {
    const preferences = this.state.open && this.state.request !== 'load' ? this.state : this.saved;
    const paused = preferences.userPaused;
    return !!(this.video && this.saved.background && this.saved.kind === 'video' && !paused &&
      this.connected && this.documentVisible && (!this.motionQuery?.matches || this.motionOverride) &&
      !this.previewPlaying && !this.videoTrim?.localFile && !this.playbackFailed);
  }
  updatePlayback() {
    const video = this.video;
    if (!video) return;
    video.playbackRate = this.playbackSpeed();
    if (!this.shouldPlayBackground()) { if (this.playPending === video) this.playInterrupted = video; video.pause(); video.hidden = true; return; }
    if (!video.paused || this.playPending) return;
    // Call play only on eligibility transitions. A rejected promise blocks
    // retries until an explicit Play action; automatic pauses never change SQL.
    this.playPending = video; this.playInterrupted = undefined;
    video.play().catch(() => {
      if (this.video !== video || this.playInterrupted === video) return;
      this.playbackFailed = true;
      video.pause(); video.hidden = true;
      this.playbackStatus.textContent = 'Video playback is unavailable. The still background remains visible.';
      this.playbackStatus.hidden = false;
      this.reportPlayback();
    }).finally(() => {
      if (this.playPending === video) this.playPending = undefined;
      const interrupted = this.playInterrupted === video;
      if (interrupted) this.playInterrupted = undefined;
      if (this.video === video && !this.shouldPlayBackground()) { video.pause(); video.hidden = true; }
      else if (interrupted) this.updatePlayback();
    });
  }
  retireVideo(video) {
    if (!video) return;
    video.pause(); video.removeAttribute('src'); video.load(); video.remove();
  }
  installBackgroundVideo(settings) {
    const video = document.createElement('video');
    video.id = 'background-video'; video.muted = true; video.loop = true; video.playsInline = true;
    video.preload = 'metadata'; video.hidden = true;
    video.src = `/visual/background?id=${encodeURIComponent(settings.backgroundID)}`;
    this.video = video; this.playbackFailed = false; this.playPending = undefined;
    if (this.playbackStatus) {this.playbackStatus.hidden = true; this.playbackStatus.textContent = '';}
    // The decoded poster remains underneath throughout loading/buffering and
    // automatic pauses. Retired element events cannot publish into a new asset.
    // The player belongs to the selected asset and survives reconnects.
    // Connection generations fence imports/decodes; playback eligibility fences
    // this reusable player's events during disconnect, visibility and pause.
    const ownsVideo = () => this.video === video;
    video.addEventListener('playing', () => { if (ownsVideo()) video.hidden = !this.shouldPlayBackground(); });
    video.addEventListener('waiting', () => { if (ownsVideo()) video.hidden = true; });
    video.addEventListener('error', () => {
      if (!ownsVideo()) return;
      this.playbackFailed = true; this.updatePlayback();
      this.playbackStatus.textContent = 'Video playback is unavailable. The still background remains visible.';
      this.playbackStatus.hidden = false;
      this.reportPlayback();
    });
    video.addEventListener('loadedmetadata', () => { if (ownsVideo()) this.renderBackground(); });
    this.surface.append(video);
  }
  releasePreviewVideo() {
    this.stopPreviewPlayback(); this.retireVideo(this.previewVideo); this.previewVideo = undefined;
  }
  stopPreviewPlayback() {
    this.previewPlaying = false;
    if (this.previewVideo) {
      this.previewVideo.pause();
      // Source scrubbing displays a paused frame, never an eligible player.
      this.previewVideo.hidden = this.previewVideo !== this.videoTrim?.sourceVideo;
    }
    this.updatePlayback();
  }
  async togglePreviewPlayback() {
    if (this.videoTrim?.localFile && this.previewVideo === this.videoTrim.sourceVideo) {
      if (!this.videoTrim.acceptedVideo) return;
      this.videoTrim.showAccepted(false);
    }
    if (this.previewPlaying) {this.stopPreviewPlayback(); return;}
    if (!this.previewVideo && this.saved.kind === 'video') {
      const video = document.createElement('video');
      video.muted = true; video.loop = true; video.playsInline = true; video.hidden = true;
      video.id = 'background-preview-video';
      video.src = `/visual/background?id=${encodeURIComponent(this.saved.backgroundID)}`;
      this.previewVideo = video; this.previewFrameElement.append(video);
      video.addEventListener('loadedmetadata', () => this.queuePreview(), {once: true});
    }
    const video = this.previewVideo;
    if (!video || !this.connected || !this.documentVisible) return;
    this.previewPlaying = true; this.updatePlayback();
    video.playbackRate = this.playbackSpeed();
    try {
      await video.play();
      if (this.previewVideo !== video || !this.state.open || !this.previewPlaying) {video.pause();return;}
      video.hidden = false; this.queuePreview();
      if (!this.playbackFailed) {
        this.playbackStatus.hidden = true;
        this.playbackStatus.textContent = '';
      }
    } catch {
      // Retiring an old player can reject its pending play promise after a new
      // preview has started. That completion must not pause the new owner.
      if (this.previewVideo === video && this.previewPlaying) {
        this.stopPreviewPlayback();
        // Playback is optional after decode/acknowledgement. Keep the accepted
        // cut applicable while explaining why the poster is showing instead.
        this.playbackStatus.textContent = this.videoTrim?.acceptedVideo === video
          ? 'Preview playback is unavailable. The prepared loop is still ready to apply.'
          : 'Preview playback is unavailable. The still preview remains visible.';
        this.playbackStatus.hidden = false;
      }
    }
  }

}
