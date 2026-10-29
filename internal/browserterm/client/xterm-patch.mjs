// Alternate-screen reflow is disabled, but its lines must still shrink to the
// viewport. Retained hidden cells break ICH/DCH right-margin editing and can
// revive stale markers on grow. Thoughts redraws its Go view after every resize.
// Match the pinned bundle exactly so upgrades require reviewing this correction.
export function trimAlternateScreenOnShrink(code, version) {
  if (version !== '6.0.0') throw new Error('Review xterm viewport patch for the new version');
  const original = 'this._isReflowEnabled && (this._reflow(e3, t3), this._cols > e3)';
  if (code.split(original).length !== 2) throw new Error('Review xterm viewport patch: resize source changed');
  return code.replace(original, '(this._isReflowEnabled && this._reflow(e3, t3), this._cols > e3)');
}

