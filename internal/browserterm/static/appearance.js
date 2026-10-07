/* Browser presentation only: no terminal transport or microphone ownership. */
class ThoughtsAppearance {
  constructor(term) {
    this.term = term;
    this.dialog = document.getElementById('appearance-dialog');
    this.image = document.getElementById('background-image');
    this.overlay = document.getElementById('background-overlay');
    this.file = document.getElementById('background-file');
    this.preview = document.getElementById('background-preview');
    this.previewWrap = document.getElementById('background-preview-wrap');
    this.slider = document.getElementById('background-darkness');
    this.status = document.getElementById('appearance-status');
    this.saved = {background: false, darkness: 70};
    this.busy = false;
    this.previewGeneration = 0;
    this.ready = this.restore();
    document.addEventListener('keydown', event => {
      if ((event.ctrlKey || event.metaKey) && event.key === ',') {
        event.preventDefault();
        event.stopImmediatePropagation();
        this.open();
      }
    }, true);
    this.file.addEventListener('change', () => this.previewFile());
    this.slider.addEventListener('input', () => this.updateDarkness());
    document.getElementById('appearance-apply').addEventListener('click', () => this.save(false));
    document.getElementById('appearance-remove').addEventListener('click', () => this.save(true));
    document.getElementById('appearance-close').addEventListener('click', () => this.dialog.close());
    this.dialog.addEventListener('cancel', event => { if (this.busy) event.preventDefault(); });
    this.dialog.addEventListener('close', () => {
      this.discardPreview();
      this.slider.value = this.saved.darkness;
      this.updateDarkness();
      this.term.focus();
    });
  }
  async open() {
    if (this.isOpen()) return;
    this.dialog.showModal();
    this.setBusy(true);
    await this.ready;
    this.slider.value = this.saved.darkness;
    if (this.activeURL) {
      this.preview.src = this.activeURL;
      this.preview.hidden = false;
      this.previewWrap.hidden = false;
    }
    this.updateDarkness();
    this.setBusy(false);
  }
  isOpen() { return this.dialog.open; }
  setBusy(value) {
    this.busy = value;
    for (const control of this.dialog.querySelectorAll('button,input')) control.disabled = value;
  }
  updateDarkness() {
    const value = Math.max(0, Math.min(95, Number(this.slider.value) || 0));
    document.getElementById('darkness-value').value = `${value}%`;
    this.previewWrap.style.setProperty('--darkness', value / 100);
    this.overlay.style.backgroundColor = `rgb(0 0 0 / ${value}%)`;
  }
  discardPreview() {
    this.previewGeneration++;
    if (this.previewURL) URL.revokeObjectURL(this.previewURL);
    this.previewURL = undefined;
    this.preview.removeAttribute('src');
    this.preview.hidden = true;
    this.previewWrap.hidden = true;
    this.file.value = '';
  }
  async previewFile() {
    const generation = ++this.previewGeneration;
    if (this.previewURL) URL.revokeObjectURL(this.previewURL);
    this.preview.hidden = true;
    this.previewWrap.hidden = true;
    const file = this.file.files[0];
    if (!file) return;
    if (file.size > 16 * 1024 * 1024) {
      this.status.textContent = 'Image exceeds 16 MiB.';
      this.file.value = '';
      return;
    }
    try {
      // The native decoder remains authoritative. A bitmap preview is static,
      // including when a selected PNG happens to contain animation chunks.
      const bytes = new Uint8Array(await file.slice(0, 8).arrayBuffer());
      const png = bytes.join(',') === '137,80,78,71,13,10,26,10';
      const jpeg = bytes[0] === 255 && bytes[1] === 216 && bytes[2] === 255;
      if (!png && !jpeg) throw new Error();
      const bitmap = await createImageBitmap(file);
      try {
        if (bitmap.width > 8192 || bitmap.height > 8192 || bitmap.width * bitmap.height > 24000000) throw new Error();
        const canvas = document.createElement('canvas');
        const scale = Math.min(400 / bitmap.width, 200 / bitmap.height, 1);
        canvas.width = Math.max(1, Math.round(bitmap.width * scale));
        canvas.height = Math.max(1, Math.round(bitmap.height * scale));
        canvas.getContext('2d').drawImage(bitmap, 0, 0, canvas.width, canvas.height);
        const blob = await new Promise(resolve => canvas.toBlob(resolve));
        if (!blob || generation !== this.previewGeneration) return;
        this.previewURL = URL.createObjectURL(blob);
        this.preview.src = this.previewURL;
        this.preview.hidden = false;
        this.previewWrap.hidden = false;
        this.status.textContent = 'Preview only. Apply to save this image.';
      } finally { bitmap.close(); }
    } catch {
      if (generation !== this.previewGeneration) return;
      this.file.value = '';
      this.status.textContent = 'Choose a JPEG or PNG within the image limits.';
    }
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
    this.slider.value = settings.darkness;
    this.updateDarkness();
  }
  async restore() {
    try {
      const response = await fetch('/appearance', {cache: 'no-store'});
      if (!response.ok) throw new Error();
      await this.activate(await response.json());
      this.status.textContent = '';
    } catch {
      this.status.textContent = 'Could not load the background. A persistent database and readable image are required.';
    }
  }
  async save(remove) {
    if (this.busy) return;
    this.setBusy(true);
    this.status.textContent = 'Saving…';
    try {
      let url = '/appearance/settings';
      let options = {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({darkness: Number(this.slider.value)})};
      if (remove) {
        url = '/appearance/background';
        options = {method: 'DELETE'};
      } else if (this.file.files[0]) {
        const form = new FormData();
        form.append('image', this.file.files[0]);
        form.append('darkness', this.slider.value);
        url = '/appearance/background';
        options = {method: 'POST', body: form};
      }
      const response = await fetch(url, options);
      if (!response.ok) throw new Error();
      const settings = await response.json();
      await this.activate(settings);
      if (settings.cleanupWarning) {
        this.discardPreview();
        this.status.textContent = 'Saved. The previous image file could not be removed.';
      } else {
        this.status.textContent = '';
        this.dialog.close();
      }
    } catch {
      // A lost response may follow a committed write. Reconcile with native
      // state before reporting failure instead of assuming that it rolled back.
      await this.restore();
      this.slider.value = this.saved.darkness;
      this.updateDarkness();
      this.status.textContent = 'Could not apply the change. Check the image limits and try again.';
    } finally { this.setBusy(false); }
  }
}
