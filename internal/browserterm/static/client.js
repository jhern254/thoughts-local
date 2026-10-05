/* Thoughts' local browser adapter. Terminal rendering and key encoding are xterm.js. */
(() => {
  'use strict';
  const container = document.getElementById('terminal');
  const status = document.getElementById('status');
  const reconnect = document.getElementById('reconnect');
  const encoder = new TextEncoder();
  const maxPaste = 1024 * 1024;
  let socket;
  let connected = false;
  let ended = false;
  let pendingResize;
  let resizeRetry;
  let voiceState = {};
  let captureIdentity;
  const microphone = new ThoughtsMicrophone(action => {
    if (!captureIdentity || !connected) return;
    sendVoice({action, ...captureIdentity});
  });
  function sendVoice(action) {
    if (send('v', encoder.encode(JSON.stringify(action)))) return true;
    // Recording controls are never replayed. End capture/session if its control
    // cannot be delivered, so the backend cannot remain incorrectly unlocked.
    clearVoice();
    if (socket) socket.close();
    return false;
  }
  function clearVoice() {
    microphone.cancel();
    captureIdentity = undefined;
    voiceState = {};
  }
  function receiveVoice(state) {
    const restoreFocus = state.draft && (voiceState.draft !== state.draft || (voiceState.state !== "idle" && state.state === "idle"));
    const identity = {draft: state.draft, recording: state.recording};
    if (!captureIdentity || captureIdentity.draft !== identity.draft || captureIdentity.recording !== identity.recording) {
      microphone.cancel();
      captureIdentity = identity.draft ? identity : undefined;
    }
    voiceState = state;
    if (state.state === 'requesting') microphone.start();
    else if (state.state === 'stopping') microphone.stop();
    else if (state.state === 'idle' || !state.draft) microphone.cancel();
    if (restoreFocus) term.focus();
  }
  window.addEventListener('pagehide', clearVoice);

  // Use xterm's supported logger seam. Never forward terminal data or errors.
  const quiet = () => {};
  const terminalFailure = () => { status.textContent = 'Terminal rendering failed. Reload to reconnect.'; };
  const term = new Terminal({
    allowProposedApi: true,
    fontFamily: 'monospace', fontSize: 14, lineHeight: 1,
    // Bubble Tea writes LF to this stream; there is no PTY applying ONLCR.
    scrollback: 0, convertEol: true,
    theme: {background: '#171717', foreground: '#eeeeee'},
    logger: {trace: quiet, debug: quiet, info: quiet, warn: terminalFailure, error: terminalFailure},
    linkHandler: {activate: quiet},
    disableStdin: true
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.loadAddon(new UnicodeGraphemesAddon.UnicodeGraphemesAddon());
  term.unicode.activeVersion = '15-graphemes';
  term.open(container);
  // No programmatic clipboard writes or reads from terminal escape sequences.
  term.parser.registerOscHandler(52, () => true);

  function send(kind, bytes) {
    if (!connected || socket.readyState !== WebSocket.OPEN) return false;
    // Bound pending input; do not replay it into a later session.
    if (socket.bufferedAmount + bytes.length > 2 * maxPaste) {
      status.textContent = 'Input not sent: connection is busy. Draft unchanged.';
      return false;
    }
    const frame = new Uint8Array(bytes.length + 1);
    frame[0] = kind.charCodeAt(0);
    frame.set(bytes, 1);
    socket.send(frame);
    return true;
  }

  function resize() {
    const size = fit.proposeDimensions();
    if (!size) return;
    const cols = Math.max(2, Math.min(512, size.cols));
    const rows = Math.max(1, Math.min(128, size.rows));
    if (term.cols !== cols || term.rows !== rows) term.resize(cols, rows);
  }
  function clearResize() {
    pendingResize = undefined;
    clearTimeout(resizeRetry);
    resizeRetry = undefined;
  }
  function flushResize(connection) {
    if (socket !== connection || !pendingResize || !connected || connection.readyState !== WebSocket.OPEN) return;
    if (send('2', encoder.encode(JSON.stringify(pendingResize)))) {
      clearResize();
    } else if (resizeRetry === undefined) {
      // WebSocket has no writable/drain event. Poll only while this session has
      // an unsent resize; newer dimensions replace it. Never retain UI input.
      resizeRetry = setTimeout(() => {
        resizeRetry = undefined;
        flushResize(connection);
      }, 50);
    }
  }
  function sendResize(size) {
    pendingResize = size;
    flushResize(socket);
  }
  term.onResize(({cols, rows}) => sendResize({cols, rows}));
  term.onData(data => send('0', encoder.encode(data)));
  new ResizeObserver(resize).observe(container);

  // Intercept the browser's paste event before xterm normalizes LF to CR or
  // removes control characters. The native editor decides whether to accept it.
  container.addEventListener('paste', event => {
    event.preventDefault();
    event.stopImmediatePropagation();
    if (!connected) return;
    if (!event.clipboardData) {
      status.textContent = 'Could not read the browser clipboard. Draft unchanged.';
      return;
    }
    const text = event.clipboardData.getData('text/plain');
    if (!text.isWellFormed()) {
      status.textContent = 'Input rejected: invalid Unicode. Draft unchanged.';
      return;
    }
    const bytes = encoder.encode(text);
    if (bytes.length > maxPaste) {
      status.textContent = 'Paste exceeds 1 MiB. Draft unchanged.';
      return;
    }
    if (send('p', bytes)) status.textContent = 'Connected';
  }, true);

  term.attachCustomKeyEventHandler(event => {
    const key = event.key.toLowerCase();
    // Let the browser deliver paste and copy selected text. With no selection,
    // Ctrl+C retains Thoughts' normal session-quit behavior.
    if (((event.ctrlKey || event.metaKey) && key === 'v') || (event.shiftKey && key === 'insert')) return false;
    if ((event.ctrlKey || event.metaKey) && key === 'c' && term.hasSelection()) return false;
    return true;
  });

  function connect(attempt = 0) {
    clearVoice();
    clearResize();
    if (socket) socket.close();
    ended = false;
    connected = false;
    term.reset();
    term.options.disableStdin = true;
    resize();
    reconnect.hidden = true;
    status.textContent = 'Connecting…';
    const connection = new WebSocket(`ws://${location.host}/ws`);
    let retryBusy = false;
    socket = connection;
    connection.binaryType = 'arraybuffer';
    connection.onopen = () => {
      if (socket !== connection) return;
      connected = true;
      sendResize({cols: term.cols, rows: term.rows});
      term.options.disableStdin = false;
      status.textContent = 'Connected';
      term.focus();
    };
    connection.onmessage = event => {
      if (socket !== connection) return;
      const frame = new Uint8Array(event.data);
      if (frame[0] === 49) {
        term.write(frame.subarray(1));
      } else if (frame[0] === 118) {
        receiveVoice(JSON.parse(new TextDecoder().decode(frame.subarray(1))));
      } else if (frame[0] === 55) {
        ended = true;
        const reason = new TextDecoder().decode(frame.subarray(1));
        // A reload can arrive while the old model's cursor command is finishing.
        // Retry only admission, for at most one second. No input or UI is replayed.
        retryBusy = reason === 'busy' && attempt < 4;
        status.textContent = retryBusy ? 'Connecting…' : ({
          busy: 'Another tab is active. Close it, then reconnect.',
          quit: 'Session ended. Reconnect to start again.',
          input: 'Unsupported terminal input. Reconnect to start again.',
          failed: 'The terminal session failed. Reconnect to start again.'
        })[reason] || 'Session ended.';
        connection.close();
      }
    };
    connection.onerror = () => {}; // onclose supplies a fixed diagnostic.
    connection.onclose = () => {
      if (socket !== connection) return;
      connected = false;
      clearVoice();
      clearResize();
      term.options.disableStdin = true;
      if (retryBusy) {
        setTimeout(() => {
          if (socket === connection) connect(attempt + 1);
        }, 250);
        return;
      }
      reconnect.hidden = false;
      if (!ended) status.textContent = 'Connection lost. Unsaved changes are not restored.';
    };
  }
  reconnect.addEventListener('click', () => connect());
  connect();
})();
