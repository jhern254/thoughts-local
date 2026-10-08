/* Bubble Tea owns controls; the browser owns the picker, image pixels and HTTP. */
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
    this.saved = {background: false, darkness: 70, framing: defaultBackgroundFraming()};
    this.state = {open: false};
    this.previewGeneration = 0;
    this.connectionGeneration = 0;
    this.ready = this.restore();
    document.addEventListener('keydown', event => {
      if ((event.ctrlKey || event.metaKey) && event.key === ',') {
        event.preventDefault();
        event.stopImmediatePropagation();
        this.sendAction({action: 'open'});
      }
    }, true);
    this.image.addEventListener('load', () => this.renderBackground());
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
  reply(action, operationID, connectionGeneration = this.connectionGeneration) {
    if (connectionGeneration === this.connectionGeneration && this.state.open && this.state.id === operationID) {
      this.sendAction({action, id: operationID, darkness: this.saved.darkness, background: this.saved.background, framing: this.saved.framing});
    }
  }
  async receive(state) {
    const previousState = this.state;
    this.state = state;
    if (!state.open || !state.editing || state.id !== previousState.id) this.cancelFramingDrag();
    this.refreshAfterClose = previousState.open && !state.open;
    if (JSON.stringify(state.preview) !== JSON.stringify(previousState.preview)) this.previewContainer.hidden = true;
    if (!state.open) {
      this.discardPreview();
      this.updateDarkness(this.saved.darkness);
      if (previousState.open) this.terminal.focus();
      return;
    }
    this.updateDarkness(state.request === 'load' ? this.saved.darkness : state.darkness);
    this.queuePreview();
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
      case 'apply': case 'remove':
        await this.save(state.request === 'remove', operationID, state.darkness, state.framing, connectionGeneration);
        break;
    }
  }
  disconnect() {
    this.connectionGeneration++;
    this.state = {open: false};
    this.refreshAfterClose = false;
    this.discardPreview();
    this.updateDarkness(this.saved.darkness);
  }
  renderBackground() {
    if (!this.saved.background || !this.image.naturalWidth) return;
    this.placeImage(this.image, window.innerWidth, window.innerHeight, this.saved.framing);
  }
  placeImage(image, width, height, framing, sourceDimensions) {
    if (!image.naturalWidth || !image.naturalHeight) return;
    const geometry = backgroundGeometry(
      sourceDimensions?.width || image.naturalWidth,
      sourceDimensions?.height || image.naturalHeight,
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
    value = Number.isInteger(value) ? Math.max(0, Math.min(95, value)) : 70;
    this.previewContainer.style.setProperty('--darkness', value / 100);
    this.overlay.style.backgroundColor = `rgb(0 0 0 / ${value}%)`;
  }
  queuePreview() {
    if (!this.state.open) { this.previewContainer.hidden = true; return; }
    cancelAnimationFrame(this.previewFrame);
    this.previewFrame = requestAnimationFrame(() => this.positionPreview());
  }
  positionPreview() {
    this.previewContainer.hidden = true;
    const previewRect = this.state.preview;
    if (!this.state.open || !previewRect || !previewRect.Height || !this.preview.getAttribute('src')) return;
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
    const scale = Math.min(availableWidth / window.innerWidth, availableHeight / window.innerHeight);
    const width = window.innerWidth * scale;
    const height = window.innerHeight * scale;
    Object.assign(this.previewFrameElement.style, {
      left: `${(availableWidth-width)/2}px`, top: `${(availableHeight-height)/2}px`, width: `${width}px`, height: `${height}px`
    });
    this.previewFrameElement.dataset.editing = String(!!this.state.editing);
    this.placeImage(this.preview, width, height, this.state.framing, this.previewSourceSize);
    this.previewContainer.hidden = false;
  }
  discardPreview() {
    this.cancelFramingDrag();
    cancelAnimationFrame(this.previewFrame);
    this.previewContainer.hidden = true;
    this.previewGeneration++;
    if (this.previewURL) URL.revokeObjectURL(this.previewURL);
    this.previewURL = undefined;
    this.previewSourceSize = undefined;
    this.preview.removeAttribute('src');
    this.file.value = '';
  }
  async previewFile() {
    const operationID = this.state.id;
    if (!this.state.open || this.state.request !== 'choose') { this.file.value = ''; return; }
    const previewGeneration = ++this.previewGeneration;
    const file = this.file.files[0];
    if (!file) {this.reply('cancelled', operationID); return;}
    try {
      if (file.size > 16 * 1024 * 1024) throw new Error();
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
        const scale = Math.min(1200 / bitmap.width, 800 / bitmap.height, 1);
        canvas.width = Math.max(1, Math.round(bitmap.width * scale));
        canvas.height = Math.max(1, Math.round(bitmap.height * scale));
        canvas.getContext('2d').drawImage(bitmap, 0, 0, canvas.width, canvas.height);
        const blob = await new Promise(resolve => canvas.toBlob(resolve));
        if (!blob || previewGeneration !== this.previewGeneration || this.state.id !== operationID) return;
        if (this.previewURL) URL.revokeObjectURL(this.previewURL);
        this.previewURL = URL.createObjectURL(blob);
        // Thumbnail rounding must not change the crop of very thin images.
        this.previewSourceSize = {width:bitmap.width, height:bitmap.height};
        this.preview.src = this.previewURL;
        this.queuePreview();
        this.reply('selected', operationID);
      } finally { bitmap.close(); }
    } catch {
      if (previewGeneration !== this.previewGeneration || this.state.id !== operationID) return;
      this.discardPreview();
      if (this.activeURL) this.preview.src = this.activeURL;
      this.reply('failed', operationID);
    } finally { this.terminal.focus(); }
  }
  async activate(settings) {
    if (!Number.isInteger(settings.darkness) || settings.darkness < 0 || settings.darkness > 95) settings.darkness = 70;
    if (settings.background) {
      const response = await fetch('/visual/background', {cache: 'no-store'});
      if (!response.ok) throw new Error();
      const url = URL.createObjectURL(await response.blob());
      const candidate = new Image();
      candidate.src = url;
      try { await candidate.decode(); } catch (error) { URL.revokeObjectURL(url); throw error; }
      if (this.activeURL) URL.revokeObjectURL(this.activeURL);
      this.activeURL = url;
      this.image.src = url;
    } else {
      if (this.activeURL) URL.revokeObjectURL(this.activeURL);
      this.activeURL = undefined;
      this.image.removeAttribute('src');
    }
    settings.framing = safeBackgroundFraming(settings.framing);
    this.saved = settings;
    this.surface.hidden = !settings.background;
    this.renderBackground();
    this.image.hidden = !settings.background;
    this.overlay.hidden = !settings.background;
    this.terminal.options.theme = {...this.terminal.options.theme, background: settings.background ? '#00000000' : '#171717'};
    this.updateDarkness(settings.darkness);
  }
  async restore() {
    try {
      const response = await fetch('/visual', {cache: 'no-store'});
      if (!response.ok) throw new Error();
      await this.activate(await response.json());
      this.loadFailed = false;
    } catch { this.loadFailed = true; }
  }
  async save(removeBackground, operationID, darkness, framing, connectionGeneration) {
    try {
      let requestURL = '/visual/settings';
      let requestOptions = {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({darkness, framing})};
      if (removeBackground) {
        requestURL = '/visual/background'; requestOptions = {method: 'DELETE'};
      } else if (this.file.files[0]) {
        const form = new FormData();
        form.append('image', this.file.files[0]); form.append('darkness', darkness);
        form.append('framing', JSON.stringify(framing));
        requestURL = '/visual/background'; requestOptions = {method: 'POST', body: form};
      }
      const response = await fetch(requestURL, requestOptions);
      if (!response.ok) throw new Error();
      const settings = await response.json();
      await this.activate(settings);
      if (settings.cleanupWarning) {
        this.discardPreview();
        if (this.activeURL) this.preview.src = this.activeURL;
      }
      this.reply(settings.cleanupWarning ? 'warning' : 'saved', operationID, connectionGeneration);
    } catch {
      // Reconcile an ambiguous lost response with authoritative native state.
      await this.restore();
      this.reply('failed', operationID, connectionGeneration);
    }
  }
}
