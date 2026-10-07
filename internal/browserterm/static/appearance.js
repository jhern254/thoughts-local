/* Bubble Tea owns controls; the browser owns the picker, image pixels and HTTP. */
class ThoughtsAppearance {
  constructor(term, send) {
    this.term = term;
    this.send = send;
    this.image = document.getElementById('background-image');
    this.overlay = document.getElementById('background-overlay');
    this.file = document.getElementById('background-file');
    this.preview = document.getElementById('background-preview');
    this.previewWrap = document.getElementById('background-preview-wrap');
    this.saved = {background: false, darkness: 70};
    this.state = {open: false};
    this.previewGeneration = 0;
    this.connectionGeneration = 0;
    this.ready = this.restore();
    document.addEventListener('keydown', event => {
      if ((event.ctrlKey || event.metaKey) && event.key === ',') {
        event.preventDefault();
        event.stopImmediatePropagation();
        this.send({action: 'open'});
      }
    }, true);
    this.file.addEventListener('change', () => this.previewFile());
    this.file.addEventListener('cancel', () => this.reply('cancelled', this.state.id));
    this.term.onRender(() => this.queuePreview());
    new ResizeObserver(() => { this.previewWrap.hidden = true; this.queuePreview(); }).observe(this.term.element);
  }
  isOpen() { return this.state.open; }
  reply(action, id, connection = this.connectionGeneration) {
    if (connection === this.connectionGeneration && this.state.open && this.state.id === id) {
      this.send({action, id, darkness: this.saved.darkness});
    }
  }
  async receive(state) {
    const previous = this.state;
    this.state = state;
    if (JSON.stringify(state.preview) !== JSON.stringify(previous.preview)) this.previewWrap.hidden = true;
    if (!state.open) {
      this.discardPreview();
      this.updateDarkness(this.saved.darkness);
      if (previous.open) this.term.focus();
      return;
    }
    this.updateDarkness(state.request === 'load' ? this.saved.darkness : state.darkness);
    this.queuePreview();
    if (state.id === previous.id && state.request === previous.request) return;
    const id = state.id;
    const connection = this.connectionGeneration;
    switch (state.request) {
      case 'load':
        await this.ready;
        if (connection !== this.connectionGeneration || !this.state.open || this.state.id !== id) return;
        if (this.activeURL) this.preview.src = this.activeURL;
        this.reply(this.loadFailed ? 'failed' : 'loaded', id);
        this.term.focus();
        break;
      case 'choose':
        // Keep the native picker attached to the existing document. No terminal
        // replacement, reconnect or microphone operation is involved.
        this.file.click();
        break;
      case 'apply': case 'remove':
        await this.save(state.request === 'remove', id, state.darkness, connection);
        break;
    }
  }
  disconnect() {
    this.connectionGeneration++;
    this.state = {open: false};
    this.discardPreview();
    this.updateDarkness(this.saved.darkness);
  }
  updateDarkness(value) {
    value = Number.isInteger(value) ? Math.max(0, Math.min(95, value)) : 70;
    this.previewWrap.style.setProperty('--darkness', value / 100);
    this.overlay.style.backgroundColor = `rgb(0 0 0 / ${value}%)`;
  }
  queuePreview() {
    if (!this.state.open) { this.previewWrap.hidden = true; return; }
    cancelAnimationFrame(this.previewFrame);
    this.previewFrame = requestAnimationFrame(() => this.positionPreview());
  }
  positionPreview() {
    this.previewWrap.hidden = true;
    const r = this.state.preview;
    if (!this.state.open || !r || !r.Height || !this.preview.getAttribute('src')) return;
    if (this.state.request === 'load' || this.term.cols < r.X + r.Width + 1 || this.term.rows < r.Y + r.Height + 1) return;
    // Metadata can arrive before Bubble Tea's next paint. Verify the reserved
    // cells in xterm's public buffer before placing pixels over them. Geometry
    // comes from the TUI; buffer reads only guard against a stale/partial paint.
    const line = y => this.term.buffer.active.getLine(y)?.translateToString(false) || '';
    if (line(r.Y - 1).slice(r.X - 1, r.X + r.Width + 1) !== '╭' + '─'.repeat(r.Width) + '╮') return;
    if (line(r.Y + r.Height).slice(r.X - 1, r.X + r.Width + 1) !== '╰' + '─'.repeat(r.Width) + '╯') return;
    for (let y = r.Y; y < r.Y + r.Height; y++) {
      if (line(y).slice(r.X - 1, r.X + r.Width + 1) !== '│' + ' '.repeat(r.Width) + '│') return;
    }
    const screen = this.term.element.querySelector('.xterm-screen').getBoundingClientRect();
    const cellWidth = screen.width / this.term.cols;
    const cellHeight = screen.height / this.term.rows;
    Object.assign(this.previewWrap.style, {
      left: `${screen.left + r.X * cellWidth}px`, top: `${screen.top + r.Y * cellHeight}px`,
      width: `${r.Width * cellWidth}px`, height: `${r.Height * cellHeight}px`
    });
    this.previewWrap.hidden = false;
  }
  discardPreview() {
    this.previewGeneration++;
    if (this.previewURL) URL.revokeObjectURL(this.previewURL);
    this.previewURL = undefined;
    this.preview.removeAttribute('src');
    this.previewWrap.hidden = true;
    this.file.value = '';
  }
  async previewFile() {
    const id = this.state.id;
    if (!this.state.open || this.state.request !== 'choose') { this.file.value = ''; return; }
    const generation = ++this.previewGeneration;
    const file = this.file.files[0];
    if (!file) {this.reply('cancelled', id); return;}
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
        if (!blob || generation !== this.previewGeneration || this.state.id !== id) return;
        if (this.previewURL) URL.revokeObjectURL(this.previewURL);
        this.previewURL = URL.createObjectURL(blob);
        this.preview.src = this.previewURL;
        this.queuePreview();
        this.reply('selected', id);
      } finally { bitmap.close(); }
    } catch {
      if (generation !== this.previewGeneration || this.state.id !== id) return;
      this.discardPreview();
      if (this.activeURL) this.preview.src = this.activeURL;
      this.reply('failed', id);
    } finally { this.term.focus(); }
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
    this.term.options.theme = {...this.term.options.theme, background: settings.background ? '#00000000' : '#171717'};
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
  async save(remove, id, darkness, connection) {
    try {
      let url = '/appearance/settings';
      let options = {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({darkness})};
      if (remove) {
        url = '/appearance/background'; options = {method: 'DELETE'};
      } else if (this.file.files[0]) {
        const form = new FormData();
        form.append('image', this.file.files[0]); form.append('darkness', darkness);
        url = '/appearance/background'; options = {method: 'POST', body: form};
      }
      const response = await fetch(url, options);
      if (!response.ok) throw new Error();
      const settings = await response.json();
      await this.activate(settings);
      if (settings.cleanupWarning) {
        this.discardPreview();
        if (this.activeURL) this.preview.src = this.activeURL;
      }
      this.reply(settings.cleanupWarning ? 'warning' : 'saved', id, connection);
    } catch {
      // Reconcile an ambiguous lost response with authoritative native state.
      await this.restore();
      this.reply('failed', id, connection);
    }
  }
}
