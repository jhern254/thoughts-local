/* Bubble Tea owns controls; the browser owns the picker, image pixels and HTTP. */
class ThoughtsAppearance {
  constructor(terminal, sendAction) {
    this.terminal = terminal;
    this.sendAction = sendAction;
    this.image = document.getElementById('background-image');
    this.overlay = document.getElementById('background-overlay');
    this.file = document.getElementById('background-file');
    this.preview = document.getElementById('background-preview');
    this.previewContainer = document.getElementById('background-preview-wrap');
    this.saved = {background: false, darkness: 70};
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
      this.sendAction({action, id: operationID, darkness: this.saved.darkness});
    }
  }
  async receive(state) {
    const previousState = this.state;
    this.state = state;
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
        await this.save(state.request === 'remove', operationID, state.darkness, connectionGeneration);
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
    this.previewContainer.hidden = false;
  }
  discardPreview() {
    cancelAnimationFrame(this.previewFrame);
    this.previewContainer.hidden = true;
    this.previewGeneration++;
    if (this.previewURL) URL.revokeObjectURL(this.previewURL);
    this.previewURL = undefined;
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
      const response = await fetch('/appearance/background', {cache: 'no-store'});
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
    this.saved = settings;
    this.image.hidden = !settings.background;
    this.overlay.hidden = !settings.background;
    this.terminal.options.theme = {...this.terminal.options.theme, background: settings.background ? '#00000000' : '#171717'};
    this.updateDarkness(settings.darkness);
  }
  async restore() {
    try {
      const response = await fetch('/appearance', {cache: 'no-store'});
      if (!response.ok) throw new Error();
      await this.activate(await response.json());
      this.loadFailed = false;
    } catch { this.loadFailed = true; }
  }
  async save(removeBackground, operationID, darkness, connectionGeneration) {
    try {
      let requestURL = '/appearance/settings';
      let requestOptions = {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({darkness})};
      if (removeBackground) {
        requestURL = '/appearance/background'; requestOptions = {method: 'DELETE'};
      } else if (this.file.files[0]) {
        const form = new FormData();
        form.append('image', this.file.files[0]); form.append('darkness', darkness);
        requestURL = '/appearance/background'; requestOptions = {method: 'POST', body: form};
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
