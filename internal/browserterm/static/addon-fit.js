!(function(e, t) {
  "object" == typeof exports && "object" == typeof module ? module.exports = t() : "function" == typeof define && define.amd ? define([], t) : "object" == typeof exports ? exports.FitAddon = t() : e.FitAddon = t();
})(globalThis, (() => (() => {
  "use strict";
  var e = {};
  return (() => {
    var t = e;
    Object.defineProperty(t, "__esModule", { value: true }), t.FitAddon = void 0, t.FitAddon = class {
      activate(e2) {
        this._terminal = e2;
      }
      dispose() {
      }
      fit() {
        const e2 = this.proposeDimensions();
        if (!e2 || !this._terminal || isNaN(e2.cols) || isNaN(e2.rows)) return;
        const t2 = this._terminal._core;
        this._terminal.rows === e2.rows && this._terminal.cols === e2.cols || (t2._renderService.clear(), this._terminal.resize(e2.cols, e2.rows));
      }
      proposeDimensions() {
        if (!this._terminal) return;
        if (!this._terminal.element || !this._terminal.element.parentElement) return;
        const e2 = this._terminal._core._renderService.dimensions;
        if (0 === e2.css.cell.width || 0 === e2.css.cell.height) return;
        const t2 = 0 === this._terminal.options.scrollback ? 0 : this._terminal.options.overviewRuler?.width || 14, r = window.getComputedStyle(this._terminal.element.parentElement), i = parseInt(r.getPropertyValue("height")), o = Math.max(0, parseInt(r.getPropertyValue("width"))), s = window.getComputedStyle(this._terminal.element), n = i - (parseInt(s.getPropertyValue("padding-top")) + parseInt(s.getPropertyValue("padding-bottom"))), l = o - (parseInt(s.getPropertyValue("padding-right")) + parseInt(s.getPropertyValue("padding-left"))) - t2;
        return { cols: Math.max(2, Math.floor(l / e2.css.cell.width)), rows: Math.max(1, Math.floor(n / e2.css.cell.height)) };
      }
    };
  })(), e;
})()));
