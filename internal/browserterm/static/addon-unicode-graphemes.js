!(function(e, t) {
  "object" == typeof exports && "object" == typeof module ? module.exports = t() : "function" == typeof define && define.amd ? define([], t) : "object" == typeof exports ? exports.UnicodeGraphemesAddon = t() : e.UnicodeGraphemesAddon = t();
})(globalThis, (() => (() => {
  "use strict";
  var e = { 106: (e2, t2, s2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.UnicodeGraphemeProvider = void 0;
    const r2 = s2(765), n = s2(200);
    class i {
      constructor(e3 = true) {
        this.ambiguousCharsAreWide = false, this.version = e3 ? "15-graphemes" : "15", this.handleGraphemes = e3;
      }
      charProperties(e3, t3) {
        if (e3 >= 32 && e3 < 127 && !(t3 >> 3)) return i._plainNarrowProperties;
        let s3 = n.getInfo(e3), o = n.infoToWidthInfo(s3), a = false;
        if (o = o >= 2 && (3 === o || this.ambiguousCharsAreWide || 65039 === e3) ? 2 : 1, 0 !== t3) {
          const e4 = r2.UnicodeService.extractWidth(t3);
          s3 = this.handleGraphemes ? n.shouldJoin(r2.UnicodeService.extractCharKind(t3), s3) : 0 === o ? 1 : 0, a = s3 > 0, a && (e4 > o ? o = e4 : 32 === s3 && (o = 2));
        }
        return r2.UnicodeService.createPropertyValue(s3, o, a);
      }
      wcwidth(e3) {
        const t3 = n.getInfo(e3), s3 = n.infoToWidthInfo(t3), r3 = (t3 & n.GRAPHEME_BREAK_MASK) >> n.GRAPHEME_BREAK_SHIFT;
        return r3 === n.GRAPHEME_BREAK_Extend || r3 === n.GRAPHEME_BREAK_Prepend ? 0 : s3 >= 2 && (3 === s3 || this.ambiguousCharsAreWide) ? 2 : 1;
      }
    }
    t2.UnicodeGraphemeProvider = i, i._plainNarrowProperties = r2.UnicodeService.createPropertyValue(n.GRAPHEME_BREAK_Other, 1, false);
  }, 200: (e2, t2, s2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.CHARWIDTH_WIDE = t2.CHARWIDTH_EA_AMBIGUOUS = t2.CHARWIDTH_FORCE_1COLUMN = t2.CHARWIDTH_NORMAL = t2.GRAPHEME_BREAK_ExtPic = t2.GRAPHEME_BREAK_ZWJ = t2.GRAPHEME_BREAK_Hangul_LVT = t2.GRAPHEME_BREAK_Hangul_LV = t2.GRAPHEME_BREAK_Hangul_T = t2.GRAPHEME_BREAK_Hangul_V = t2.GRAPHEME_BREAK_Hangul_L = t2.GRAPHEME_BREAK_SpacingMark = t2.GRAPHEME_BREAK_Regional_Indicator = t2.GRAPHEME_BREAK_Extend = t2.GRAPHEME_BREAK_Prepend = t2.GRAPHEME_BREAK_Other = t2.CHARWIDTH_SHIFT = t2.CHARWIDTH_MASK = t2.GRAPHEME_BREAK_SHIFT = t2.GRAPHEME_BREAK_MASK = void 0, t2.infoToWidthInfo = i, t2.infoToWidth = o, t2.strWidth = function(e3, t3) {
      let s3 = 0;
      for (let r3 = 0; r3 < e3.length; ) {
        const n2 = e3.codePointAt(r3);
        s3 += o(l(n2), t3), r3 += n2 <= 65535 ? 1 : 2;
      }
      return s3;
    }, t2.columnToIndexInContext = function(e3, t3, s3, r3) {
      let n2 = 0;
      for (let i2 = t3; ; ) {
        if (i2 >= e3.length) return i2;
        const t4 = e3.codePointAt(i2);
        if (n2 += o(l(t4), r3), n2 > s3) return i2;
        i2 += t4 <= 65535 ? 1 : 2;
      }
    }, t2.shouldJoin = function(e3, s3) {
      let r3 = (e3 & t2.GRAPHEME_BREAK_MASK) >> t2.GRAPHEME_BREAK_SHIFT, i2 = (s3 & t2.GRAPHEME_BREAK_MASK) >> t2.GRAPHEME_BREAK_SHIFT;
      return a(r3, i2) ? i2 === t2.GRAPHEME_BREAK_Regional_Indicator ? n : i2 + 16 : i2 - 16;
    }, t2.shouldJoinBackwards = function(e3, s3) {
      let r3 = (s3 & t2.GRAPHEME_BREAK_MASK) >> t2.GRAPHEME_BREAK_SHIFT, i2 = (e3 & t2.GRAPHEME_BREAK_MASK) >> t2.GRAPHEME_BREAK_SHIFT;
      return a(i2, r3) ? i2 === t2.GRAPHEME_BREAK_Regional_Indicator ? n : i2 + 16 : i2 - 16;
    }, t2.getInfo = l;
    const r2 = new (s2(38)).default((function(e3) {
      if ("undefined" != typeof Buffer) return Buffer.from(e3, "base64");
      const t3 = atob(e3), s3 = new Uint8Array(t3.length);
      for (let e4 = 0; e4 < s3.length; ++e4) s3[e4] = t3.charCodeAt(e4);
      return s3;
    })("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"));
    t2.GRAPHEME_BREAK_MASK = 15, t2.GRAPHEME_BREAK_SHIFT = 0, t2.CHARWIDTH_MASK = 48, t2.CHARWIDTH_SHIFT = 4, t2.GRAPHEME_BREAK_Other = 0, t2.GRAPHEME_BREAK_Prepend = 1, t2.GRAPHEME_BREAK_Extend = 2, t2.GRAPHEME_BREAK_Regional_Indicator = 3, t2.GRAPHEME_BREAK_SpacingMark = 4, t2.GRAPHEME_BREAK_Hangul_L = 5, t2.GRAPHEME_BREAK_Hangul_V = 6, t2.GRAPHEME_BREAK_Hangul_T = 7, t2.GRAPHEME_BREAK_Hangul_LV = 8, t2.GRAPHEME_BREAK_Hangul_LVT = 9, t2.GRAPHEME_BREAK_ZWJ = 10, t2.GRAPHEME_BREAK_ExtPic = 11;
    const n = 32;
    function i(e3) {
      return (e3 & t2.CHARWIDTH_MASK) >> t2.CHARWIDTH_SHIFT;
    }
    function o(e3, s3 = false) {
      const r3 = i(e3);
      return r3 < t2.CHARWIDTH_EA_AMBIGUOUS ? 1 : r3 >= t2.CHARWIDTH_WIDE || s3 ? 2 : 1;
    }
    function a(e3, s3) {
      if (e3 >= t2.GRAPHEME_BREAK_Hangul_L && e3 <= t2.GRAPHEME_BREAK_Hangul_LVT) {
        if (e3 == t2.GRAPHEME_BREAK_Hangul_L && (s3 == t2.GRAPHEME_BREAK_Hangul_L || s3 == t2.GRAPHEME_BREAK_Hangul_V || s3 == t2.GRAPHEME_BREAK_Hangul_LV || s3 == t2.GRAPHEME_BREAK_Hangul_LVT)) return true;
        if (!(e3 != t2.GRAPHEME_BREAK_Hangul_LV && e3 != t2.GRAPHEME_BREAK_Hangul_V || s3 != t2.GRAPHEME_BREAK_Hangul_V && s3 != t2.GRAPHEME_BREAK_Hangul_T)) return true;
        if ((e3 == t2.GRAPHEME_BREAK_Hangul_LVT || e3 == t2.GRAPHEME_BREAK_Hangul_T) && s3 == t2.GRAPHEME_BREAK_Hangul_T) return true;
      }
      return s3 == t2.GRAPHEME_BREAK_Extend || s3 == t2.GRAPHEME_BREAK_ZWJ || e3 == t2.GRAPHEME_BREAK_Prepend || s3 == t2.GRAPHEME_BREAK_SpacingMark || e3 == t2.GRAPHEME_BREAK_ZWJ && s3 == t2.GRAPHEME_BREAK_ExtPic || s3 == t2.GRAPHEME_BREAK_Regional_Indicator && e3 == t2.GRAPHEME_BREAK_Regional_Indicator;
    }
    function l(e3) {
      return r2.get(e3);
    }
    t2.CHARWIDTH_NORMAL = 0, t2.CHARWIDTH_FORCE_1COLUMN = 1, t2.CHARWIDTH_EA_AMBIGUOUS = 2, t2.CHARWIDTH_WIDE = 3;
  }, 12: (e2, t2) => {
    Object.defineProperty(t2, "__esModule", { value: true });
    class s2 {
      constructor() {
        this.table = new Uint16Array(16), this.trans = new Uint16Array(288);
      }
    }
    class r2 {
      constructor(e3, t3) {
        this.tag = 0, this.bitcount = 0, this.destLen = 0, this.sourceIndex = 0, this.source = e3, this.dest = t3, this.ltree = new s2(), this.dtree = new s2();
      }
    }
    var n = new s2(), i = new s2(), o = new Uint8Array(30), a = new Uint16Array(30), l = new Uint8Array(30), c = new Uint16Array(30), u = new Uint8Array([16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15]);
    const h = new s2(), d = new Uint8Array(320);
    function f(e3, t3, s3, r3) {
      var n2, i2;
      for (n2 = 0; n2 < s3; ++n2) e3[n2] = 0;
      for (n2 = 0; n2 < 30 - s3; ++n2) e3[n2 + s3] = n2 / s3 | 0;
      for (i2 = r3, n2 = 0; n2 < 30; ++n2) t3[n2] = i2, i2 += 1 << e3[n2];
    }
    var p = new Uint16Array(16);
    function _(e3, t3, s3, r3) {
      var n2, i2;
      for (n2 = 0; n2 < 16; ++n2) e3.table[n2] = 0;
      for (n2 = 0; n2 < r3; ++n2) e3.table[t3[s3 + n2]]++;
      for (e3.table[0] = 0, i2 = 0, n2 = 0; n2 < 16; ++n2) p[n2] = i2, i2 += e3.table[n2];
      for (n2 = 0; n2 < r3; ++n2) t3[s3 + n2] && (e3.trans[p[t3[s3 + n2]]++] = n2);
    }
    function v(e3) {
      e3.bitcount-- || (e3.tag = e3.source[e3.sourceIndex++], e3.bitcount = 7);
      var t3 = 1 & e3.tag;
      return e3.tag >>>= 1, t3;
    }
    function E(e3, t3, s3) {
      if (!t3) return s3;
      for (; e3.bitcount < 24; ) e3.tag |= e3.source[e3.sourceIndex++] << e3.bitcount, e3.bitcount += 8;
      var r3 = e3.tag & 65535 >>> 16 - t3;
      return e3.tag >>>= t3, e3.bitcount -= t3, r3 + s3;
    }
    function g(e3, t3) {
      for (; e3.bitcount < 24; ) e3.tag |= e3.source[e3.sourceIndex++] << e3.bitcount, e3.bitcount += 8;
      var s3 = 0, r3 = 0, n2 = 0, i2 = e3.tag;
      do {
        r3 = 2 * r3 + (1 & i2), i2 >>>= 1, ++n2, s3 += t3.table[n2], r3 -= t3.table[n2];
      } while (r3 >= 0);
      return e3.tag = i2, e3.bitcount -= n2, t3.trans[s3 + r3];
    }
    function m(e3, t3, s3) {
      var r3, n2, i2, o2, a2, l2;
      for (r3 = E(e3, 5, 257), n2 = E(e3, 5, 1), i2 = E(e3, 4, 4), o2 = 0; o2 < 19; ++o2) d[o2] = 0;
      for (o2 = 0; o2 < i2; ++o2) {
        var c2 = E(e3, 3, 0);
        d[u[o2]] = c2;
      }
      for (_(h, d, 0, 19), a2 = 0; a2 < r3 + n2; ) {
        var f2 = g(e3, h);
        switch (f2) {
          case 16:
            var p2 = d[a2 - 1];
            for (l2 = E(e3, 2, 3); l2; --l2) d[a2++] = p2;
            break;
          case 17:
            for (l2 = E(e3, 3, 3); l2; --l2) d[a2++] = 0;
            break;
          case 18:
            for (l2 = E(e3, 7, 11); l2; --l2) d[a2++] = 0;
            break;
          default:
            d[a2++] = f2;
        }
      }
      _(t3, d, 0, r3), _(s3, d, r3, n2);
    }
    function b(e3, t3, s3) {
      for (; ; ) {
        var r3, n2, i2, u2, h2 = g(e3, t3);
        if (256 === h2) return 0;
        if (h2 < 256) e3.dest[e3.destLen++] = h2;
        else for (r3 = E(e3, o[h2 -= 257], a[h2]), n2 = g(e3, s3), u2 = i2 = e3.destLen - E(e3, l[n2], c[n2]); u2 < i2 + r3; ++u2) e3.dest[e3.destLen++] = e3.dest[u2];
      }
    }
    function A(e3) {
      for (var t3, s3; e3.bitcount > 8; ) e3.sourceIndex--, e3.bitcount -= 8;
      if ((t3 = 256 * (t3 = e3.source[e3.sourceIndex + 1]) + e3.source[e3.sourceIndex]) !== (65535 & ~(256 * e3.source[e3.sourceIndex + 3] + e3.source[e3.sourceIndex + 2]))) return -3;
      for (e3.sourceIndex += 4, s3 = t3; s3; --s3) e3.dest[e3.destLen++] = e3.source[e3.sourceIndex++];
      return e3.bitcount = 0, 0;
    }
    !(function(e3, t3) {
      var s3;
      for (s3 = 0; s3 < 7; ++s3) e3.table[s3] = 0;
      for (e3.table[7] = 24, e3.table[8] = 152, e3.table[9] = 112, s3 = 0; s3 < 24; ++s3) e3.trans[s3] = 256 + s3;
      for (s3 = 0; s3 < 144; ++s3) e3.trans[24 + s3] = s3;
      for (s3 = 0; s3 < 8; ++s3) e3.trans[168 + s3] = 280 + s3;
      for (s3 = 0; s3 < 112; ++s3) e3.trans[176 + s3] = 144 + s3;
      for (s3 = 0; s3 < 5; ++s3) t3.table[s3] = 0;
      for (t3.table[5] = 32, s3 = 0; s3 < 32; ++s3) t3.trans[s3] = s3;
    })(n, i), f(o, a, 4, 3), f(l, c, 2, 1), o[28] = 0, a[28] = 258, t2.default = function(e3, t3) {
      var s3, o2, a2 = new r2(e3, t3);
      do {
        switch (s3 = v(a2), E(a2, 2, 0)) {
          case 0:
            o2 = A(a2);
            break;
          case 1:
            o2 = b(a2, n, i);
            break;
          case 2:
            m(a2, a2.ltree, a2.dtree), o2 = b(a2, a2.ltree, a2.dtree);
            break;
          default:
            o2 = -3;
        }
        if (0 !== o2) throw new Error("Data error");
      } while (!s3);
      return a2.destLen < a2.dest.length ? "function" == typeof a2.dest.slice ? a2.dest.slice(0, a2.destLen) : a2.dest.subarray(0, a2.destLen) : a2.dest;
    };
  }, 38: (e2, t2, s2) => {
    Object.defineProperty(t2, "__esModule", { value: true });
    const r2 = s2(12), n = 18 === new Uint8Array(new Uint32Array([305419896]).buffer)[0];
    t2.default = class {
      constructor(e3) {
        const t3 = new DataView(e3.buffer);
        this.highStart = t3.getUint32(0, true), this.errorValue = t3.getUint32(4, true);
        let s3 = t3.getUint32(8, true);
        if (e3 = e3.subarray(12), e3 = (0, r2.default)(e3, new Uint8Array(s3)), e3 = (0, r2.default)(e3, new Uint8Array(s3)), n) {
          const t4 = e3.length;
          for (let s4 = 0; s4 < t4; s4 += 4) {
            let t5 = e3[s4];
            e3[s4] = e3[s4 + 3], e3[s4 + 3] = t5;
            let r3 = e3[s4 + 1];
            e3[s4 + 1] = e3[s4 + 2], e3[s4 + 2] = r3;
          }
        }
        this.data = new Uint32Array(e3.buffer);
      }
      get(e3) {
        let t3;
        return e3 < 0 || e3 > 1114111 ? this.errorValue : e3 < 55296 || e3 > 56319 && e3 <= 65535 ? (t3 = (this.data[e3 >> 5] << 2) + (31 & e3), this.data[t3]) : e3 <= 65535 ? (t3 = (this.data[2048 + (e3 - 55296 >> 5)] << 2) + (31 & e3), this.data[t3]) : e3 < this.highStart ? (t3 = this.data[2080 + (e3 >> 11)], t3 = this.data[t3 + (e3 >> 5 & 63)], t3 = (t3 << 2) + (31 & e3), this.data[t3]) : this.data[this.data.length - 4];
      }
    };
  }, 546: (e2, t2, s2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.UnicodeV6 = void 0;
    const r2 = s2(765), n = [[768, 879], [1155, 1158], [1160, 1161], [1425, 1469], [1471, 1471], [1473, 1474], [1476, 1477], [1479, 1479], [1536, 1539], [1552, 1557], [1611, 1630], [1648, 1648], [1750, 1764], [1767, 1768], [1770, 1773], [1807, 1807], [1809, 1809], [1840, 1866], [1958, 1968], [2027, 2035], [2305, 2306], [2364, 2364], [2369, 2376], [2381, 2381], [2385, 2388], [2402, 2403], [2433, 2433], [2492, 2492], [2497, 2500], [2509, 2509], [2530, 2531], [2561, 2562], [2620, 2620], [2625, 2626], [2631, 2632], [2635, 2637], [2672, 2673], [2689, 2690], [2748, 2748], [2753, 2757], [2759, 2760], [2765, 2765], [2786, 2787], [2817, 2817], [2876, 2876], [2879, 2879], [2881, 2883], [2893, 2893], [2902, 2902], [2946, 2946], [3008, 3008], [3021, 3021], [3134, 3136], [3142, 3144], [3146, 3149], [3157, 3158], [3260, 3260], [3263, 3263], [3270, 3270], [3276, 3277], [3298, 3299], [3393, 3395], [3405, 3405], [3530, 3530], [3538, 3540], [3542, 3542], [3633, 3633], [3636, 3642], [3655, 3662], [3761, 3761], [3764, 3769], [3771, 3772], [3784, 3789], [3864, 3865], [3893, 3893], [3895, 3895], [3897, 3897], [3953, 3966], [3968, 3972], [3974, 3975], [3984, 3991], [3993, 4028], [4038, 4038], [4141, 4144], [4146, 4146], [4150, 4151], [4153, 4153], [4184, 4185], [4448, 4607], [4959, 4959], [5906, 5908], [5938, 5940], [5970, 5971], [6002, 6003], [6068, 6069], [6071, 6077], [6086, 6086], [6089, 6099], [6109, 6109], [6155, 6157], [6313, 6313], [6432, 6434], [6439, 6440], [6450, 6450], [6457, 6459], [6679, 6680], [6912, 6915], [6964, 6964], [6966, 6970], [6972, 6972], [6978, 6978], [7019, 7027], [7616, 7626], [7678, 7679], [8203, 8207], [8234, 8238], [8288, 8291], [8298, 8303], [8400, 8431], [12330, 12335], [12441, 12442], [43014, 43014], [43019, 43019], [43045, 43046], [64286, 64286], [65024, 65039], [65056, 65059], [65279, 65279], [65529, 65531]], i = [[68097, 68099], [68101, 68102], [68108, 68111], [68152, 68154], [68159, 68159], [119143, 119145], [119155, 119170], [119173, 119179], [119210, 119213], [119362, 119364], [917505, 917505], [917536, 917631], [917760, 917999]];
    let o;
    t2.UnicodeV6 = class {
      constructor() {
        if (this.version = "6", !o) {
          o = new Uint8Array(65536), o.fill(1), o[0] = 0, o.fill(0, 1, 32), o.fill(0, 127, 160), o.fill(2, 4352, 4448), o[9001] = 2, o[9002] = 2, o.fill(2, 11904, 42192), o[12351] = 1, o.fill(2, 44032, 55204), o.fill(2, 63744, 64256), o.fill(2, 65040, 65050), o.fill(2, 65072, 65136), o.fill(2, 65280, 65377), o.fill(2, 65504, 65511);
          for (let e3 = 0; e3 < n.length; ++e3) o.fill(0, n[e3][0], n[e3][1] + 1);
        }
      }
      wcwidth(e3) {
        return e3 < 32 ? 0 : e3 < 127 ? 1 : e3 < 65536 ? o[e3] : (function(e4, t3) {
          let s3, r3 = 0, n2 = t3.length - 1;
          if (e4 < t3[0][0] || e4 > t3[n2][1]) return false;
          for (; n2 >= r3; ) if (s3 = r3 + n2 >> 1, e4 > t3[s3][1]) r3 = s3 + 1;
          else {
            if (!(e4 < t3[s3][0])) return true;
            n2 = s3 - 1;
          }
          return false;
        })(e3, i) ? 0 : e3 >= 131072 && e3 <= 196605 || e3 >= 196608 && e3 <= 262141 ? 2 : 1;
      }
      charProperties(e3, t3) {
        let s3 = this.wcwidth(e3), n2 = 0 === s3 && 0 !== t3;
        if (n2) {
          const e4 = r2.UnicodeService.extractWidth(t3);
          0 === e4 ? n2 = false : e4 > s3 && (s3 = e4);
        }
        return r2.UnicodeService.createPropertyValue(0, s3, n2);
      }
    };
  }, 765: (e2, t2, s2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.UnicodeService = void 0;
    const r2 = s2(546), n = s2(276);
    class i {
      static extractShouldJoin(e3) {
        return !!(1 & e3);
      }
      static extractWidth(e3) {
        return e3 >> 1 & 3;
      }
      static extractCharKind(e3) {
        return e3 >> 3;
      }
      static createPropertyValue(e3, t3, s3 = false) {
        return (16777215 & e3) << 3 | (3 & t3) << 1 | (s3 ? 1 : 0);
      }
      constructor() {
        this._providers = /* @__PURE__ */ Object.create(null), this._active = "", this._onChange = new n.Emitter(), this.onChange = this._onChange.event;
        const e3 = new r2.UnicodeV6();
        this.register(e3), this._active = e3.version, this._activeProvider = e3;
      }
      dispose() {
        this._onChange.dispose();
      }
      get versions() {
        return Object.keys(this._providers);
      }
      get activeVersion() {
        return this._active;
      }
      set activeVersion(e3) {
        if (!this._providers[e3]) throw new Error(`unknown Unicode version "${e3}"`);
        this._active = e3, this._activeProvider = this._providers[e3], this._onChange.fire(e3);
      }
      register(e3) {
        this._providers[e3.version] = e3;
      }
      wcwidth(e3) {
        return this._activeProvider.wcwidth(e3);
      }
      getStringCellWidth(e3) {
        let t3 = 0, s3 = 0;
        const r3 = e3.length;
        for (let n2 = 0; n2 < r3; ++n2) {
          let o = e3.charCodeAt(n2);
          if (55296 <= o && o <= 56319) {
            if (++n2 >= r3) return t3 + this.wcwidth(o);
            const s4 = e3.charCodeAt(n2);
            56320 <= s4 && s4 <= 57343 ? o = 1024 * (o - 55296) + s4 - 56320 + 65536 : t3 += this.wcwidth(s4);
          }
          const a = this.charProperties(o, s3);
          let l = i.extractWidth(a);
          i.extractShouldJoin(a) && (l -= i.extractWidth(s3)), t3 += l, s3 = a;
        }
        return t3;
      }
      charProperties(e3, t3) {
        return this._activeProvider.charProperties(e3, t3);
      }
    }
    t2.UnicodeService = i;
  }, 732: (e2, t2, s2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.Permutation = t2.CallbackIterable = t2.ArrayQueue = t2.booleanComparator = t2.numberComparator = t2.CompareResult = void 0, t2.tail = function(e3, t3 = 0) {
      return e3[e3.length - (1 + t3)];
    }, t2.tail2 = function(e3) {
      if (0 === e3.length) throw new Error("Invalid tail call");
      return [e3.slice(0, e3.length - 1), e3[e3.length - 1]];
    }, t2.equals = function(e3, t3, s3 = (e4, t4) => e4 === t4) {
      if (e3 === t3) return true;
      if (!e3 || !t3) return false;
      if (e3.length !== t3.length) return false;
      for (let r3 = 0, n2 = e3.length; r3 < n2; r3++) if (!s3(e3[r3], t3[r3])) return false;
      return true;
    }, t2.removeFastWithoutKeepingOrder = function(e3, t3) {
      const s3 = e3.length - 1;
      t3 < s3 && (e3[t3] = e3[s3]), e3.pop();
    }, t2.binarySearch = function(e3, t3, s3) {
      return i(e3.length, ((r3) => s3(e3[r3], t3)));
    }, t2.binarySearch2 = i, t2.quickSelect = function e3(t3, s3, r3) {
      if ((t3 |= 0) >= s3.length) throw new TypeError("invalid index");
      const n2 = s3[Math.floor(s3.length * Math.random())], i2 = [], o2 = [], a2 = [];
      for (const e4 of s3) {
        const t4 = r3(e4, n2);
        t4 < 0 ? i2.push(e4) : t4 > 0 ? o2.push(e4) : a2.push(e4);
      }
      return t3 < i2.length ? e3(t3, i2, r3) : t3 < i2.length + a2.length ? a2[0] : e3(t3 - (i2.length + a2.length), o2, r3);
    }, t2.groupBy = function(e3, t3) {
      const s3 = [];
      let r3;
      for (const n2 of e3.slice(0).sort(t3)) r3 && 0 === t3(r3[0], n2) ? r3.push(n2) : (r3 = [n2], s3.push(r3));
      return s3;
    }, t2.groupAdjacentBy = function* (e3, t3) {
      let s3, r3;
      for (const n2 of e3) void 0 !== r3 && t3(r3, n2) ? s3.push(n2) : (s3 && (yield s3), s3 = [n2]), r3 = n2;
      s3 && (yield s3);
    }, t2.forEachAdjacent = function(e3, t3) {
      for (let s3 = 0; s3 <= e3.length; s3++) t3(0 === s3 ? void 0 : e3[s3 - 1], s3 === e3.length ? void 0 : e3[s3]);
    }, t2.forEachWithNeighbors = function(e3, t3) {
      for (let s3 = 0; s3 < e3.length; s3++) t3(0 === s3 ? void 0 : e3[s3 - 1], e3[s3], s3 + 1 === e3.length ? void 0 : e3[s3 + 1]);
    }, t2.sortedDiff = o, t2.delta = function(e3, t3, s3) {
      const r3 = o(e3, t3, s3), n2 = [], i2 = [];
      for (const t4 of r3) n2.push(...e3.slice(t4.start, t4.start + t4.deleteCount)), i2.push(...t4.toInsert);
      return { removed: n2, added: i2 };
    }, t2.top = function(e3, t3, s3) {
      if (0 === s3) return [];
      const r3 = e3.slice(0, s3).sort(t3);
      return a(e3, t3, r3, s3, e3.length), r3;
    }, t2.topAsync = function(e3, t3, s3, n2, i2) {
      return 0 === s3 ? Promise.resolve([]) : new Promise(((o2, l2) => {
        (async () => {
          const o3 = e3.length, l3 = e3.slice(0, s3).sort(t3);
          for (let c2 = s3, u2 = Math.min(s3 + n2, o3); c2 < o3; c2 = u2, u2 = Math.min(u2 + n2, o3)) {
            if (c2 > s3 && await new Promise(((e4) => setTimeout(e4))), i2 && i2.isCancellationRequested) throw new r2.CancellationError();
            a(e3, t3, l3, c2, u2);
          }
          return l3;
        })().then(o2, l2);
      }));
    }, t2.coalesce = function(e3) {
      return e3.filter(((e4) => !!e4));
    }, t2.coalesceInPlace = function(e3) {
      let t3 = 0;
      for (let s3 = 0; s3 < e3.length; s3++) e3[s3] && (e3[t3] = e3[s3], t3 += 1);
      e3.length = t3;
    }, t2.move = function(e3, t3, s3) {
      e3.splice(s3, 0, e3.splice(t3, 1)[0]);
    }, t2.isFalsyOrEmpty = function(e3) {
      return !Array.isArray(e3) || 0 === e3.length;
    }, t2.isNonEmptyArray = function(e3) {
      return Array.isArray(e3) && e3.length > 0;
    }, t2.distinct = function(e3, t3 = (e4) => e4) {
      const s3 = /* @__PURE__ */ new Set();
      return e3.filter(((e4) => {
        const r3 = t3(e4);
        return !s3.has(r3) && (s3.add(r3), true);
      }));
    }, t2.uniqueFilter = function(e3) {
      const t3 = /* @__PURE__ */ new Set();
      return (s3) => {
        const r3 = e3(s3);
        return !t3.has(r3) && (t3.add(r3), true);
      };
    }, t2.firstOrDefault = function(e3, t3) {
      return e3.length > 0 ? e3[0] : t3;
    }, t2.lastOrDefault = function(e3, t3) {
      return e3.length > 0 ? e3[e3.length - 1] : t3;
    }, t2.commonPrefixLength = function(e3, t3, s3 = (e4, t4) => e4 === t4) {
      let r3 = 0;
      for (let n2 = 0, i2 = Math.min(e3.length, t3.length); n2 < i2 && s3(e3[n2], t3[n2]); n2++) r3++;
      return r3;
    }, t2.range = function(e3, t3) {
      let s3 = "number" == typeof t3 ? e3 : 0;
      "number" == typeof t3 ? s3 = e3 : (s3 = 0, t3 = e3);
      const r3 = [];
      if (s3 <= t3) for (let e4 = s3; e4 < t3; e4++) r3.push(e4);
      else for (let e4 = s3; e4 > t3; e4--) r3.push(e4);
      return r3;
    }, t2.index = function(e3, t3, s3) {
      return e3.reduce(((e4, r3) => (e4[t3(r3)] = s3 ? s3(r3) : r3, e4)), /* @__PURE__ */ Object.create(null));
    }, t2.insert = function(e3, t3) {
      return e3.push(t3), () => l(e3, t3);
    }, t2.remove = l, t2.arrayInsert = function(e3, t3, s3) {
      const r3 = e3.slice(0, t3), n2 = e3.slice(t3);
      return r3.concat(s3, n2);
    }, t2.shuffle = function(e3, t3) {
      let s3;
      if ("number" == typeof t3) {
        let e4 = t3;
        s3 = () => {
          const t4 = 179426549 * Math.sin(e4++);
          return t4 - Math.floor(t4);
        };
      } else s3 = Math.random;
      for (let t4 = e3.length - 1; t4 > 0; t4 -= 1) {
        const r3 = Math.floor(s3() * (t4 + 1)), n2 = e3[t4];
        e3[t4] = e3[r3], e3[r3] = n2;
      }
    }, t2.pushToStart = function(e3, t3) {
      const s3 = e3.indexOf(t3);
      s3 > -1 && (e3.splice(s3, 1), e3.unshift(t3));
    }, t2.pushToEnd = function(e3, t3) {
      const s3 = e3.indexOf(t3);
      s3 > -1 && (e3.splice(s3, 1), e3.push(t3));
    }, t2.pushMany = function(e3, t3) {
      for (const s3 of t3) e3.push(s3);
    }, t2.mapArrayOrNot = function(e3, t3) {
      return Array.isArray(e3) ? e3.map(t3) : t3(e3);
    }, t2.asArray = function(e3) {
      return Array.isArray(e3) ? e3 : [e3];
    }, t2.getRandomElement = function(e3) {
      return e3[Math.floor(Math.random() * e3.length)];
    }, t2.insertInto = c, t2.splice = function(e3, t3, s3, r3) {
      const n2 = u(e3, t3);
      let i2 = e3.splice(n2, s3);
      return void 0 === i2 && (i2 = []), c(e3, n2, r3), i2;
    }, t2.compareBy = function(e3, t3) {
      return (s3, r3) => t3(e3(s3), e3(r3));
    }, t2.tieBreakComparators = function(...e3) {
      return (t3, s3) => {
        for (const r3 of e3) {
          const e4 = r3(t3, s3);
          if (!h.isNeitherLessOrGreaterThan(e4)) return e4;
        }
        return h.neitherLessOrGreaterThan;
      };
    }, t2.reverseOrder = function(e3) {
      return (t3, s3) => -e3(t3, s3);
    };
    const r2 = s2(577), n = s2(411);
    function i(e3, t3) {
      let s3 = 0, r3 = e3 - 1;
      for (; s3 <= r3; ) {
        const e4 = (s3 + r3) / 2 | 0, n2 = t3(e4);
        if (n2 < 0) s3 = e4 + 1;
        else {
          if (!(n2 > 0)) return e4;
          r3 = e4 - 1;
        }
      }
      return -(s3 + 1);
    }
    function o(e3, t3, s3) {
      const r3 = [];
      function n2(e4, t4, s4) {
        if (0 === t4 && 0 === s4.length) return;
        const n3 = r3[r3.length - 1];
        n3 && n3.start + n3.deleteCount === e4 ? (n3.deleteCount += t4, n3.toInsert.push(...s4)) : r3.push({ start: e4, deleteCount: t4, toInsert: s4 });
      }
      let i2 = 0, o2 = 0;
      for (; ; ) {
        if (i2 === e3.length) {
          n2(i2, 0, t3.slice(o2));
          break;
        }
        if (o2 === t3.length) {
          n2(i2, e3.length - i2, []);
          break;
        }
        const r4 = e3[i2], a2 = t3[o2], l2 = s3(r4, a2);
        0 === l2 ? (i2 += 1, o2 += 1) : l2 < 0 ? (n2(i2, 1, []), i2 += 1) : l2 > 0 && (n2(i2, 0, [a2]), o2 += 1);
      }
      return r3;
    }
    function a(e3, t3, s3, r3, i2) {
      for (const o2 = s3.length; r3 < i2; r3++) {
        const i3 = e3[r3];
        if (t3(i3, s3[o2 - 1]) < 0) {
          s3.pop();
          const e4 = (0, n.findFirstIdxMonotonousOrArrLen)(s3, ((e5) => t3(i3, e5) < 0));
          s3.splice(e4, 0, i3);
        }
      }
    }
    function l(e3, t3) {
      const s3 = e3.indexOf(t3);
      if (s3 > -1) return e3.splice(s3, 1), t3;
    }
    function c(e3, t3, s3) {
      const r3 = u(e3, t3), n2 = e3.length, i2 = s3.length;
      e3.length = n2 + i2;
      for (let t4 = n2 - 1; t4 >= r3; t4--) e3[t4 + i2] = e3[t4];
      for (let t4 = 0; t4 < i2; t4++) e3[t4 + r3] = s3[t4];
    }
    function u(e3, t3) {
      return t3 < 0 ? Math.max(t3 + e3.length, 0) : Math.min(t3, e3.length);
    }
    var h;
    !(function(e3) {
      e3.isLessThan = function(e4) {
        return e4 < 0;
      }, e3.isLessThanOrEqual = function(e4) {
        return e4 <= 0;
      }, e3.isGreaterThan = function(e4) {
        return e4 > 0;
      }, e3.isNeitherLessOrGreaterThan = function(e4) {
        return 0 === e4;
      }, e3.greaterThan = 1, e3.lessThan = -1, e3.neitherLessOrGreaterThan = 0;
    })(h || (t2.CompareResult = h = {})), t2.numberComparator = (e3, t3) => e3 - t3, t2.booleanComparator = (e3, s3) => (0, t2.numberComparator)(e3 ? 1 : 0, s3 ? 1 : 0), t2.ArrayQueue = class {
      constructor(e3) {
        this.items = e3, this.firstIdx = 0, this.lastIdx = this.items.length - 1;
      }
      get length() {
        return this.lastIdx - this.firstIdx + 1;
      }
      takeWhile(e3) {
        let t3 = this.firstIdx;
        for (; t3 < this.items.length && e3(this.items[t3]); ) t3++;
        const s3 = t3 === this.firstIdx ? null : this.items.slice(this.firstIdx, t3);
        return this.firstIdx = t3, s3;
      }
      takeFromEndWhile(e3) {
        let t3 = this.lastIdx;
        for (; t3 >= 0 && e3(this.items[t3]); ) t3--;
        const s3 = t3 === this.lastIdx ? null : this.items.slice(t3 + 1, this.lastIdx + 1);
        return this.lastIdx = t3, s3;
      }
      peek() {
        if (0 !== this.length) return this.items[this.firstIdx];
      }
      peekLast() {
        if (0 !== this.length) return this.items[this.lastIdx];
      }
      dequeue() {
        const e3 = this.items[this.firstIdx];
        return this.firstIdx++, e3;
      }
      removeLast() {
        const e3 = this.items[this.lastIdx];
        return this.lastIdx--, e3;
      }
      takeCount(e3) {
        const t3 = this.items.slice(this.firstIdx, this.firstIdx + e3);
        return this.firstIdx += e3, t3;
      }
    };
    class d {
      static {
        this.empty = new d(((e3) => {
        }));
      }
      constructor(e3) {
        this.iterate = e3;
      }
      forEach(e3) {
        this.iterate(((t3) => (e3(t3), true)));
      }
      toArray() {
        const e3 = [];
        return this.iterate(((t3) => (e3.push(t3), true))), e3;
      }
      filter(e3) {
        return new d(((t3) => this.iterate(((s3) => !e3(s3) || t3(s3)))));
      }
      map(e3) {
        return new d(((t3) => this.iterate(((s3) => t3(e3(s3))))));
      }
      some(e3) {
        let t3 = false;
        return this.iterate(((s3) => (t3 = e3(s3), !t3))), t3;
      }
      findFirst(e3) {
        let t3;
        return this.iterate(((s3) => !e3(s3) || (t3 = s3, false))), t3;
      }
      findLast(e3) {
        let t3;
        return this.iterate(((s3) => (e3(s3) && (t3 = s3), true))), t3;
      }
      findLastMaxBy(e3) {
        let t3, s3 = true;
        return this.iterate(((r3) => ((s3 || h.isGreaterThan(e3(r3, t3))) && (s3 = false, t3 = r3), true))), t3;
      }
    }
    t2.CallbackIterable = d;
    class f {
      constructor(e3) {
        this._indexMap = e3;
      }
      static createSortPermutation(e3, t3) {
        const s3 = Array.from(e3.keys()).sort(((s4, r3) => t3(e3[s4], e3[r3])));
        return new f(s3);
      }
      apply(e3) {
        return e3.map(((t3, s3) => e3[this._indexMap[s3]]));
      }
      inverse() {
        const e3 = this._indexMap.slice();
        for (let t3 = 0; t3 < this._indexMap.length; t3++) e3[this._indexMap[t3]] = t3;
        return new f(e3);
      }
    }
    t2.Permutation = f;
  }, 411: (e2, t2) => {
    function s2(e3, t3, s3 = e3.length - 1) {
      for (let r3 = s3; r3 >= 0; r3--) if (t3(e3[r3])) return r3;
      return -1;
    }
    function r2(e3, t3, s3 = 0, r3 = e3.length) {
      let n2 = s3, i2 = r3;
      for (; n2 < i2; ) {
        const s4 = Math.floor((n2 + i2) / 2);
        t3(e3[s4]) ? n2 = s4 + 1 : i2 = s4;
      }
      return n2 - 1;
    }
    function n(e3, t3, s3 = 0, r3 = e3.length) {
      let n2 = s3, i2 = r3;
      for (; n2 < i2; ) {
        const s4 = Math.floor((n2 + i2) / 2);
        t3(e3[s4]) ? i2 = s4 : n2 = s4 + 1;
      }
      return n2;
    }
    Object.defineProperty(t2, "__esModule", { value: true }), t2.MonotonousArray = void 0, t2.findLast = function(e3, t3) {
      const r3 = s2(e3, t3);
      if (-1 !== r3) return e3[r3];
    }, t2.findLastIdx = s2, t2.findLastMonotonous = function(e3, t3) {
      const s3 = r2(e3, t3);
      return -1 === s3 ? void 0 : e3[s3];
    }, t2.findLastIdxMonotonous = r2, t2.findFirstMonotonous = function(e3, t3) {
      const s3 = n(e3, t3);
      return s3 === e3.length ? void 0 : e3[s3];
    }, t2.findFirstIdxMonotonousOrArrLen = n, t2.findFirstIdxMonotonous = function(e3, t3, s3 = 0, r3 = e3.length) {
      const i2 = n(e3, t3, s3, r3);
      return i2 === e3.length ? -1 : i2;
    }, t2.findFirstMax = o, t2.findLastMax = function(e3, t3) {
      if (0 === e3.length) return;
      let s3 = e3[0];
      for (let r3 = 1; r3 < e3.length; r3++) {
        const n2 = e3[r3];
        t3(n2, s3) >= 0 && (s3 = n2);
      }
      return s3;
    }, t2.findFirstMin = function(e3, t3) {
      return o(e3, ((e4, s3) => -t3(e4, s3)));
    }, t2.findMaxIdx = function(e3, t3) {
      if (0 === e3.length) return -1;
      let s3 = 0;
      for (let r3 = 1; r3 < e3.length; r3++) t3(e3[r3], e3[s3]) > 0 && (s3 = r3);
      return s3;
    }, t2.mapFindFirst = function(e3, t3) {
      for (const s3 of e3) {
        const e4 = t3(s3);
        if (void 0 !== e4) return e4;
      }
    };
    class i {
      static {
        this.assertInvariants = false;
      }
      constructor(e3) {
        this._array = e3, this._findLastMonotonousLastIdx = 0;
      }
      findLastMonotonous(e3) {
        if (i.assertInvariants) {
          if (this._prevFindLastPredicate) {
            for (const t4 of this._array) if (this._prevFindLastPredicate(t4) && !e3(t4)) throw new Error("MonotonousArray: current predicate must be weaker than (or equal to) the previous predicate.");
          }
          this._prevFindLastPredicate = e3;
        }
        const t3 = r2(this._array, e3, this._findLastMonotonousLastIdx);
        return this._findLastMonotonousLastIdx = t3 + 1, -1 === t3 ? void 0 : this._array[t3];
      }
    }
    function o(e3, t3) {
      if (0 === e3.length) return;
      let s3 = e3[0];
      for (let r3 = 1; r3 < e3.length; r3++) {
        const n2 = e3[r3];
        t3(n2, s3) > 0 && (s3 = n2);
      }
      return s3;
    }
    t2.MonotonousArray = i;
  }, 33: (e2, t2) => {
    var s2;
    Object.defineProperty(t2, "__esModule", { value: true }), t2.SetWithKey = void 0, t2.groupBy = function(e3, t3) {
      const s3 = /* @__PURE__ */ Object.create(null);
      for (const r3 of e3) {
        const e4 = t3(r3);
        let n = s3[e4];
        n || (n = s3[e4] = []), n.push(r3);
      }
      return s3;
    }, t2.diffSets = function(e3, t3) {
      const s3 = [], r3 = [];
      for (const r4 of e3) t3.has(r4) || s3.push(r4);
      for (const s4 of t3) e3.has(s4) || r3.push(s4);
      return { removed: s3, added: r3 };
    }, t2.diffMaps = function(e3, t3) {
      const s3 = [], r3 = [];
      for (const [r4, n] of e3) t3.has(r4) || s3.push(n);
      for (const [s4, n] of t3) e3.has(s4) || r3.push(n);
      return { removed: s3, added: r3 };
    }, t2.intersection = function(e3, t3) {
      const s3 = /* @__PURE__ */ new Set();
      for (const r3 of t3) e3.has(r3) && s3.add(r3);
      return s3;
    };
    class r2 {
      static {
        s2 = Symbol.toStringTag;
      }
      constructor(e3, t3) {
        this.toKey = t3, this._map = /* @__PURE__ */ new Map(), this[s2] = "SetWithKey";
        for (const t4 of e3) this.add(t4);
      }
      get size() {
        return this._map.size;
      }
      add(e3) {
        const t3 = this.toKey(e3);
        return this._map.set(t3, e3), this;
      }
      delete(e3) {
        return this._map.delete(this.toKey(e3));
      }
      has(e3) {
        return this._map.has(this.toKey(e3));
      }
      *entries() {
        for (const e3 of this._map.values()) yield [e3, e3];
      }
      keys() {
        return this.values();
      }
      *values() {
        for (const e3 of this._map.values()) yield e3;
      }
      clear() {
        this._map.clear();
      }
      forEach(e3, t3) {
        this._map.forEach(((s3) => e3.call(t3, s3, s3, this)));
      }
      [Symbol.iterator]() {
        return this.values();
      }
    }
    t2.SetWithKey = r2;
  }, 577: (e2, t2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.BugIndicatingError = t2.ErrorNoTelemetry = t2.ExpectedError = t2.NotSupportedError = t2.NotImplementedError = t2.ReadonlyError = t2.CancellationError = t2.errorHandler = t2.ErrorHandler = void 0, t2.setUnexpectedErrorHandler = function(e3) {
      t2.errorHandler.setUnexpectedErrorHandler(e3);
    }, t2.isSigPipeError = function(e3) {
      if (!e3 || "object" != typeof e3) return false;
      const t3 = e3;
      return "EPIPE" === t3.code && "WRITE" === t3.syscall?.toUpperCase();
    }, t2.onUnexpectedError = function(e3) {
      n(e3) || t2.errorHandler.onUnexpectedError(e3);
    }, t2.onUnexpectedExternalError = function(e3) {
      n(e3) || t2.errorHandler.onUnexpectedExternalError(e3);
    }, t2.transformErrorForSerialization = function(e3) {
      if (e3 instanceof Error) {
        const { name: t3, message: s3 } = e3;
        return { $isError: true, name: t3, message: s3, stack: e3.stacktrace || e3.stack, noTelemetry: u.isErrorNoTelemetry(e3) };
      }
      return e3;
    }, t2.transformErrorFromSerialization = function(e3) {
      let t3;
      return e3.noTelemetry ? t3 = new u() : (t3 = new Error(), t3.name = e3.name), t3.message = e3.message, t3.stack = e3.stack, t3;
    }, t2.isCancellationError = n, t2.canceled = function() {
      const e3 = new Error(r2);
      return e3.name = e3.message, e3;
    }, t2.illegalArgument = function(e3) {
      return e3 ? new Error(`Illegal argument: ${e3}`) : new Error("Illegal argument");
    }, t2.illegalState = function(e3) {
      return e3 ? new Error(`Illegal state: ${e3}`) : new Error("Illegal state");
    }, t2.getErrorMessage = function(e3) {
      return e3 ? e3.message ? e3.message : e3.stack ? e3.stack.split("\n")[0] : String(e3) : "Error";
    };
    class s2 {
      constructor() {
        this.listeners = [], this.unexpectedErrorHandler = function(e3) {
          setTimeout((() => {
            if (e3.stack) {
              if (u.isErrorNoTelemetry(e3)) throw new u(e3.message + "\n\n" + e3.stack);
              throw new Error(e3.message + "\n\n" + e3.stack);
            }
            throw e3;
          }), 0);
        };
      }
      addListener(e3) {
        return this.listeners.push(e3), () => {
          this._removeListener(e3);
        };
      }
      emit(e3) {
        this.listeners.forEach(((t3) => {
          t3(e3);
        }));
      }
      _removeListener(e3) {
        this.listeners.splice(this.listeners.indexOf(e3), 1);
      }
      setUnexpectedErrorHandler(e3) {
        this.unexpectedErrorHandler = e3;
      }
      getUnexpectedErrorHandler() {
        return this.unexpectedErrorHandler;
      }
      onUnexpectedError(e3) {
        this.unexpectedErrorHandler(e3), this.emit(e3);
      }
      onUnexpectedExternalError(e3) {
        this.unexpectedErrorHandler(e3);
      }
    }
    t2.ErrorHandler = s2, t2.errorHandler = new s2();
    const r2 = "Canceled";
    function n(e3) {
      return e3 instanceof i || e3 instanceof Error && e3.name === r2 && e3.message === r2;
    }
    class i extends Error {
      constructor() {
        super(r2), this.name = this.message;
      }
    }
    t2.CancellationError = i;
    class o extends TypeError {
      constructor(e3) {
        super(e3 ? `${e3} is read-only and cannot be changed` : "Cannot change read-only property");
      }
    }
    t2.ReadonlyError = o;
    class a extends Error {
      constructor(e3) {
        super("NotImplemented"), e3 && (this.message = e3);
      }
    }
    t2.NotImplementedError = a;
    class l extends Error {
      constructor(e3) {
        super("NotSupported"), e3 && (this.message = e3);
      }
    }
    t2.NotSupportedError = l;
    class c extends Error {
      constructor() {
        super(...arguments), this.isExpected = true;
      }
    }
    t2.ExpectedError = c;
    class u extends Error {
      constructor(e3) {
        super(e3), this.name = "CodeExpectedError";
      }
      static fromError(e3) {
        if (e3 instanceof u) return e3;
        const t3 = new u();
        return t3.message = e3.message, t3.stack = e3.stack, t3;
      }
      static isErrorNoTelemetry(e3) {
        return "CodeExpectedError" === e3.name;
      }
    }
    t2.ErrorNoTelemetry = u;
    class h extends Error {
      constructor(e3) {
        super(e3 || "An unexpected bug occurred."), Object.setPrototypeOf(this, h.prototype);
      }
    }
    t2.BugIndicatingError = h;
  }, 276: (e2, t2, s2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.ValueWithChangeEvent = t2.Relay = t2.EventBufferer = t2.DynamicListEventMultiplexer = t2.EventMultiplexer = t2.MicrotaskEmitter = t2.DebounceEmitter = t2.PauseableEmitter = t2.AsyncEmitter = t2.createEventDeliveryQueue = t2.Emitter = t2.ListenerRefusalError = t2.ListenerLeakError = t2.EventProfiling = t2.Event = void 0, t2.setGlobalLeakWarningThreshold = function(e3) {
      const t3 = u;
      return u = e3, { dispose() {
        u = t3;
      } };
    };
    const r2 = s2(577), n = s2(355), i = s2(540), o = s2(711), a = s2(79);
    var l;
    !(function(e3) {
      function t3(e4) {
        return (t4, s4 = null, r4) => {
          let n3, i2 = false;
          return n3 = e4(((e5) => {
            if (!i2) return n3 ? n3.dispose() : i2 = true, t4.call(s4, e5);
          }), null, r4), i2 && n3.dispose(), n3;
        };
      }
      function s3(e4, t4, s4) {
        return n2(((s5, r4 = null, n3) => e4(((e5) => s5.call(r4, t4(e5))), null, n3)), s4);
      }
      function r3(e4, t4, s4) {
        return n2(((s5, r4 = null, n3) => e4(((e5) => t4(e5) && s5.call(r4, e5)), null, n3)), s4);
      }
      function n2(e4, t4) {
        let s4;
        const r4 = new E({ onWillAddFirstListener() {
          s4 = e4(r4.fire, r4);
        }, onDidRemoveLastListener() {
          s4?.dispose();
        } });
        return t4?.add(r4), r4.event;
      }
      function o2(e4, t4, s4 = 100, r4 = false, n3 = false, i2, o3) {
        let a3, l3, c3, u2, h2 = 0;
        const d2 = new E({ leakWarningThreshold: i2, onWillAddFirstListener() {
          a3 = e4(((e5) => {
            h2++, l3 = t4(l3, e5), r4 && !c3 && (d2.fire(l3), l3 = void 0), u2 = () => {
              const e6 = l3;
              l3 = void 0, c3 = void 0, (!r4 || h2 > 1) && d2.fire(e6), h2 = 0;
            }, "number" == typeof s4 ? (clearTimeout(c3), c3 = setTimeout(u2, s4)) : void 0 === c3 && (c3 = 0, queueMicrotask(u2));
          }));
        }, onWillRemoveListener() {
          n3 && h2 > 0 && u2?.();
        }, onDidRemoveLastListener() {
          u2 = void 0, a3.dispose();
        } });
        return o3?.add(d2), d2.event;
      }
      e3.None = () => i.Disposable.None, e3.defer = function(e4, t4) {
        return o2(e4, (() => {
        }), 0, void 0, true, void 0, t4);
      }, e3.once = t3, e3.map = s3, e3.forEach = function(e4, t4, s4) {
        return n2(((s5, r4 = null, n3) => e4(((e5) => {
          t4(e5), s5.call(r4, e5);
        }), null, n3)), s4);
      }, e3.filter = r3, e3.signal = function(e4) {
        return e4;
      }, e3.any = function(...e4) {
        return (t4, s4 = null, r4) => {
          return n3 = (0, i.combinedDisposable)(...e4.map(((e5) => e5(((e6) => t4.call(s4, e6)))))), (o3 = r4) instanceof Array ? o3.push(n3) : o3 && o3.add(n3), n3;
          var n3, o3;
        };
      }, e3.reduce = function(e4, t4, r4, n3) {
        let i2 = r4;
        return s3(e4, ((e5) => (i2 = t4(i2, e5), i2)), n3);
      }, e3.debounce = o2, e3.accumulate = function(t4, s4 = 0, r4) {
        return e3.debounce(t4, ((e4, t5) => e4 ? (e4.push(t5), e4) : [t5]), s4, void 0, true, void 0, r4);
      }, e3.latch = function(e4, t4 = (e5, t5) => e5 === t5, s4) {
        let n3, i2 = true;
        return r3(e4, ((e5) => {
          const s5 = i2 || !t4(e5, n3);
          return i2 = false, n3 = e5, s5;
        }), s4);
      }, e3.split = function(t4, s4, r4) {
        return [e3.filter(t4, s4, r4), e3.filter(t4, ((e4) => !s4(e4)), r4)];
      }, e3.buffer = function(e4, t4 = false, s4 = [], r4) {
        let n3 = s4.slice(), i2 = e4(((e5) => {
          n3 ? n3.push(e5) : a3.fire(e5);
        }));
        r4 && r4.add(i2);
        const o3 = () => {
          n3?.forEach(((e5) => a3.fire(e5))), n3 = null;
        }, a3 = new E({ onWillAddFirstListener() {
          i2 || (i2 = e4(((e5) => a3.fire(e5))), r4 && r4.add(i2));
        }, onDidAddFirstListener() {
          n3 && (t4 ? setTimeout(o3) : o3());
        }, onDidRemoveLastListener() {
          i2 && i2.dispose(), i2 = null;
        } });
        return r4 && r4.add(a3), a3.event;
      }, e3.chain = function(e4, t4) {
        return (s4, r4, n3) => {
          const i2 = t4(new l2());
          return e4((function(e5) {
            const t5 = i2.evaluate(e5);
            t5 !== a2 && s4.call(r4, t5);
          }), void 0, n3);
        };
      };
      const a2 = /* @__PURE__ */ Symbol("HaltChainable");
      class l2 {
        constructor() {
          this.steps = [];
        }
        map(e4) {
          return this.steps.push(e4), this;
        }
        forEach(e4) {
          return this.steps.push(((t4) => (e4(t4), t4))), this;
        }
        filter(e4) {
          return this.steps.push(((t4) => e4(t4) ? t4 : a2)), this;
        }
        reduce(e4, t4) {
          let s4 = t4;
          return this.steps.push(((t5) => (s4 = e4(s4, t5), s4))), this;
        }
        latch(e4 = (e5, t4) => e5 === t4) {
          let t4, s4 = true;
          return this.steps.push(((r4) => {
            const n3 = s4 || !e4(r4, t4);
            return s4 = false, t4 = r4, n3 ? r4 : a2;
          })), this;
        }
        evaluate(e4) {
          for (const t4 of this.steps) if ((e4 = t4(e4)) === a2) break;
          return e4;
        }
      }
      e3.fromNodeEventEmitter = function(e4, t4, s4 = (e5) => e5) {
        const r4 = (...e5) => n3.fire(s4(...e5)), n3 = new E({ onWillAddFirstListener: () => e4.on(t4, r4), onDidRemoveLastListener: () => e4.removeListener(t4, r4) });
        return n3.event;
      }, e3.fromDOMEventEmitter = function(e4, t4, s4 = (e5) => e5) {
        const r4 = (...e5) => n3.fire(s4(...e5)), n3 = new E({ onWillAddFirstListener: () => e4.addEventListener(t4, r4), onDidRemoveLastListener: () => e4.removeEventListener(t4, r4) });
        return n3.event;
      }, e3.toPromise = function(e4) {
        return new Promise(((s4) => t3(e4)(s4)));
      }, e3.fromPromise = function(e4) {
        const t4 = new E();
        return e4.then(((e5) => {
          t4.fire(e5);
        }), (() => {
          t4.fire(void 0);
        })).finally((() => {
          t4.dispose();
        })), t4.event;
      }, e3.forward = function(e4, t4) {
        return e4(((e5) => t4.fire(e5)));
      }, e3.runAndSubscribe = function(e4, t4, s4) {
        return t4(s4), e4(((e5) => t4(e5)));
      };
      class c2 {
        constructor(e4, t4) {
          this._observable = e4, this._counter = 0, this._hasChanged = false;
          const s4 = { onWillAddFirstListener: () => {
            e4.addObserver(this);
          }, onDidRemoveLastListener: () => {
            e4.removeObserver(this);
          } };
          this.emitter = new E(s4), t4 && t4.add(this.emitter);
        }
        beginUpdate(e4) {
          this._counter++;
        }
        handlePossibleChange(e4) {
        }
        handleChange(e4, t4) {
          this._hasChanged = true;
        }
        endUpdate(e4) {
          this._counter--, 0 === this._counter && (this._observable.reportChanges(), this._hasChanged && (this._hasChanged = false, this.emitter.fire(this._observable.get())));
        }
      }
      e3.fromObservable = function(e4, t4) {
        return new c2(e4, t4).emitter.event;
      }, e3.fromObservableLight = function(e4) {
        return (t4, s4, r4) => {
          let n3 = 0, o3 = false;
          const a3 = { beginUpdate() {
            n3++;
          }, endUpdate() {
            n3--, 0 === n3 && (e4.reportChanges(), o3 && (o3 = false, t4.call(s4)));
          }, handlePossibleChange() {
          }, handleChange() {
            o3 = true;
          } };
          e4.addObserver(a3), e4.reportChanges();
          const l3 = { dispose() {
            e4.removeObserver(a3);
          } };
          return r4 instanceof i.DisposableStore ? r4.add(l3) : Array.isArray(r4) && r4.push(l3), l3;
        };
      };
    })(l || (t2.Event = l = {}));
    class c {
      static {
        this.all = /* @__PURE__ */ new Set();
      }
      static {
        this._idPool = 0;
      }
      constructor(e3) {
        this.listenerCount = 0, this.invocationCount = 0, this.elapsedOverall = 0, this.durations = [], this.name = `${e3}_${c._idPool++}`, c.all.add(this);
      }
      start(e3) {
        this._stopWatch = new a.StopWatch(), this.listenerCount = e3;
      }
      stop() {
        if (this._stopWatch) {
          const e3 = this._stopWatch.elapsed();
          this.durations.push(e3), this.elapsedOverall += e3, this.invocationCount += 1, this._stopWatch = void 0;
        }
      }
    }
    t2.EventProfiling = c;
    let u = -1;
    class h {
      static {
        this._idPool = 1;
      }
      constructor(e3, t3, s3 = (h._idPool++).toString(16).padStart(3, "0")) {
        this._errorHandler = e3, this.threshold = t3, this.name = s3, this._warnCountdown = 0;
      }
      dispose() {
        this._stacks?.clear();
      }
      check(e3, t3) {
        const s3 = this.threshold;
        if (s3 <= 0 || t3 < s3) return;
        this._stacks || (this._stacks = /* @__PURE__ */ new Map());
        const r3 = this._stacks.get(e3.value) || 0;
        if (this._stacks.set(e3.value, r3 + 1), this._warnCountdown -= 1, this._warnCountdown <= 0) {
          this._warnCountdown = 0.5 * s3;
          const [e4, r4] = this.getMostFrequentStack(), n2 = `[${this.name}] potential listener LEAK detected, having ${t3} listeners already. MOST frequent listener (${r4}):`;
          void 0, void 0;
          const i2 = new f(n2, e4);
          this._errorHandler(i2);
        }
        return () => {
          const t4 = this._stacks.get(e3.value) || 0;
          this._stacks.set(e3.value, t4 - 1);
        };
      }
      getMostFrequentStack() {
        if (!this._stacks) return;
        let e3, t3 = 0;
        for (const [s3, r3] of this._stacks) (!e3 || t3 < r3) && (e3 = [s3, r3], t3 = r3);
        return e3;
      }
    }
    class d {
      static create() {
        const e3 = new Error();
        return new d(e3.stack ?? "");
      }
      constructor(e3) {
        this.value = e3;
      }
      print() {
      }
    }
    class f extends Error {
      constructor(e3, t3) {
        super(e3), this.name = "ListenerLeakError", this.stack = t3;
      }
    }
    t2.ListenerLeakError = f;
    class p extends Error {
      constructor(e3, t3) {
        super(e3), this.name = "ListenerRefusalError", this.stack = t3;
      }
    }
    t2.ListenerRefusalError = p;
    let _ = 0;
    class v {
      constructor(e3) {
        this.value = e3, this.id = _++;
      }
    }
    class E {
      constructor(e3) {
        this._size = 0, this._options = e3, this._leakageMon = u > 0 || this._options?.leakWarningThreshold ? new h(e3?.onListenerError ?? r2.onUnexpectedError, this._options?.leakWarningThreshold ?? u) : void 0, this._perfMon = this._options?._profName ? new c(this._options._profName) : void 0, this._deliveryQueue = this._options?.deliveryQueue;
      }
      dispose() {
        this._disposed || (this._disposed = true, this._deliveryQueue?.current === this && this._deliveryQueue.reset(), this._listeners && (this._listeners = void 0, this._size = 0), this._options?.onDidRemoveLastListener?.(), this._leakageMon?.dispose());
      }
      get event() {
        return this._event ??= (e3, t3, s3) => {
          if (this._leakageMon && this._size > this._leakageMon.threshold ** 2) {
            const e4 = `[${this._leakageMon.name}] REFUSES to accept new listeners because it exceeded its threshold by far (${this._size} vs ${this._leakageMon.threshold})`;
            const t4 = this._leakageMon.getMostFrequentStack() ?? ["UNKNOWN stack", -1], s4 = new p(`${e4}. HINT: Stack shows most frequent listener (${t4[1]}-times)`, t4[0]);
            return (this._options?.onListenerError || r2.onUnexpectedError)(s4), i.Disposable.None;
          }
          if (this._disposed) return i.Disposable.None;
          t3 && (e3 = e3.bind(t3));
          const n2 = new v(e3);
          let o2;
          this._leakageMon && this._size >= Math.ceil(0.2 * this._leakageMon.threshold) && (n2.stack = d.create(), o2 = this._leakageMon.check(n2.stack, this._size + 1)), this._listeners ? this._listeners instanceof v ? (this._deliveryQueue ??= new g(), this._listeners = [this._listeners, n2]) : this._listeners.push(n2) : (this._options?.onWillAddFirstListener?.(this), this._listeners = n2, this._options?.onDidAddFirstListener?.(this)), this._size++;
          const a2 = (0, i.toDisposable)((() => {
            o2?.(), this._removeListener(n2);
          }));
          return s3 instanceof i.DisposableStore ? s3.add(a2) : Array.isArray(s3) && s3.push(a2), a2;
        }, this._event;
      }
      _removeListener(e3) {
        if (this._options?.onWillRemoveListener?.(this), !this._listeners) return;
        if (1 === this._size) return this._listeners = void 0, this._options?.onDidRemoveLastListener?.(this), void (this._size = 0);
        const t3 = this._listeners, s3 = t3.indexOf(e3);
        if (-1 === s3) throw void 0, void 0, void 0, new Error("Attempted to dispose unknown listener");
        this._size--, t3[s3] = void 0;
        const r3 = this._deliveryQueue.current === this;
        if (2 * this._size <= t3.length) {
          let e4 = 0;
          for (let s4 = 0; s4 < t3.length; s4++) t3[s4] ? t3[e4++] = t3[s4] : r3 && (this._deliveryQueue.end--, e4 < this._deliveryQueue.i && this._deliveryQueue.i--);
          t3.length = e4;
        }
      }
      _deliver(e3, t3) {
        if (!e3) return;
        const s3 = this._options?.onListenerError || r2.onUnexpectedError;
        if (s3) try {
          e3.value(t3);
        } catch (e4) {
          s3(e4);
        }
        else e3.value(t3);
      }
      _deliverQueue(e3) {
        const t3 = e3.current._listeners;
        for (; e3.i < e3.end; ) this._deliver(t3[e3.i++], e3.value);
        e3.reset();
      }
      fire(e3) {
        if (this._deliveryQueue?.current && (this._deliverQueue(this._deliveryQueue), this._perfMon?.stop()), this._perfMon?.start(this._size), this._listeners) if (this._listeners instanceof v) this._deliver(this._listeners, e3);
        else {
          const t3 = this._deliveryQueue;
          t3.enqueue(this, e3, this._listeners.length), this._deliverQueue(t3);
        }
        this._perfMon?.stop();
      }
      hasListeners() {
        return this._size > 0;
      }
    }
    t2.Emitter = E, t2.createEventDeliveryQueue = () => new g();
    class g {
      constructor() {
        this.i = -1, this.end = 0;
      }
      enqueue(e3, t3, s3) {
        this.i = 0, this.end = s3, this.current = e3, this.value = t3;
      }
      reset() {
        this.i = this.end, this.current = void 0, this.value = void 0;
      }
    }
    t2.AsyncEmitter = class extends E {
      async fireAsync(e3, t3, s3) {
        if (this._listeners) for (this._asyncDeliveryQueue || (this._asyncDeliveryQueue = new o.LinkedList()), ((e4, t4) => {
          if (e4 instanceof v) t4(e4);
          else for (let s4 = 0; s4 < e4.length; s4++) {
            const r3 = e4[s4];
            r3 && t4(r3);
          }
        })(this._listeners, ((t4) => this._asyncDeliveryQueue.push([t4.value, e3]))); this._asyncDeliveryQueue.size > 0 && !t3.isCancellationRequested; ) {
          const [e4, n2] = this._asyncDeliveryQueue.shift(), i2 = [], o2 = { ...n2, token: t3, waitUntil: (t4) => {
            if (Object.isFrozen(i2)) throw new Error("waitUntil can NOT be called asynchronous");
            s3 && (t4 = s3(t4, e4)), i2.push(t4);
          } };
          try {
            e4(o2);
          } catch (e5) {
            (0, r2.onUnexpectedError)(e5);
            continue;
          }
          Object.freeze(i2), await Promise.allSettled(i2).then(((e5) => {
            for (const t4 of e5) "rejected" === t4.status && (0, r2.onUnexpectedError)(t4.reason);
          }));
        }
      }
    };
    class m extends E {
      get isPaused() {
        return 0 !== this._isPaused;
      }
      constructor(e3) {
        super(e3), this._isPaused = 0, this._eventQueue = new o.LinkedList(), this._mergeFn = e3?.merge;
      }
      pause() {
        this._isPaused++;
      }
      resume() {
        if (0 !== this._isPaused && 0 == --this._isPaused) if (this._mergeFn) {
          if (this._eventQueue.size > 0) {
            const e3 = Array.from(this._eventQueue);
            this._eventQueue.clear(), super.fire(this._mergeFn(e3));
          }
        } else for (; !this._isPaused && 0 !== this._eventQueue.size; ) super.fire(this._eventQueue.shift());
      }
      fire(e3) {
        this._size && (0 !== this._isPaused ? this._eventQueue.push(e3) : super.fire(e3));
      }
    }
    t2.PauseableEmitter = m, t2.DebounceEmitter = class extends m {
      constructor(e3) {
        super(e3), this._delay = e3.delay ?? 100;
      }
      fire(e3) {
        this._handle || (this.pause(), this._handle = setTimeout((() => {
          this._handle = void 0, this.resume();
        }), this._delay)), super.fire(e3);
      }
    }, t2.MicrotaskEmitter = class extends E {
      constructor(e3) {
        super(e3), this._queuedEvents = [], this._mergeFn = e3?.merge;
      }
      fire(e3) {
        this.hasListeners() && (this._queuedEvents.push(e3), 1 === this._queuedEvents.length && queueMicrotask((() => {
          this._mergeFn ? super.fire(this._mergeFn(this._queuedEvents)) : this._queuedEvents.forEach(((e4) => super.fire(e4))), this._queuedEvents = [];
        })));
      }
    };
    class b {
      constructor() {
        this.hasListeners = false, this.events = [], this.emitter = new E({ onWillAddFirstListener: () => this.onFirstListenerAdd(), onDidRemoveLastListener: () => this.onLastListenerRemove() });
      }
      get event() {
        return this.emitter.event;
      }
      add(e3) {
        const t3 = { event: e3, listener: null };
        return this.events.push(t3), this.hasListeners && this.hook(t3), (0, i.toDisposable)((0, n.createSingleCallFunction)((() => {
          this.hasListeners && this.unhook(t3);
          const e4 = this.events.indexOf(t3);
          this.events.splice(e4, 1);
        })));
      }
      onFirstListenerAdd() {
        this.hasListeners = true, this.events.forEach(((e3) => this.hook(e3)));
      }
      onLastListenerRemove() {
        this.hasListeners = false, this.events.forEach(((e3) => this.unhook(e3)));
      }
      hook(e3) {
        e3.listener = e3.event(((e4) => this.emitter.fire(e4)));
      }
      unhook(e3) {
        e3.listener?.dispose(), e3.listener = null;
      }
      dispose() {
        this.emitter.dispose();
        for (const e3 of this.events) e3.listener?.dispose();
        this.events = [];
      }
    }
    t2.EventMultiplexer = b, t2.DynamicListEventMultiplexer = class {
      constructor(e3, t3, s3, r3) {
        this._store = new i.DisposableStore();
        const n2 = this._store.add(new b()), o2 = this._store.add(new i.DisposableMap());
        function a2(e4) {
          o2.set(e4, n2.add(r3(e4)));
        }
        for (const t4 of e3) a2(t4);
        this._store.add(t3(((e4) => {
          a2(e4);
        }))), this._store.add(s3(((e4) => {
          o2.deleteAndDispose(e4);
        }))), this.event = n2.event;
      }
      dispose() {
        this._store.dispose();
      }
    }, t2.EventBufferer = class {
      constructor() {
        this.data = [];
      }
      wrapEvent(e3, t3, s3) {
        return (r3, n2, i2) => e3(((e4) => {
          const i3 = this.data[this.data.length - 1];
          if (!t3) return void (i3 ? i3.buffers.push((() => r3.call(n2, e4))) : r3.call(n2, e4));
          const o2 = i3;
          o2 ? (o2.items ??= [], o2.items.push(e4), 0 === o2.buffers.length && i3.buffers.push((() => {
            o2.reducedResult ??= s3 ? o2.items.reduce(t3, s3) : o2.items.reduce(t3), r3.call(n2, o2.reducedResult);
          }))) : r3.call(n2, t3(s3, e4));
        }), void 0, i2);
      }
      bufferEvents(e3) {
        const t3 = { buffers: new Array() };
        this.data.push(t3);
        const s3 = e3();
        return this.data.pop(), t3.buffers.forEach(((e4) => e4())), s3;
      }
    }, t2.Relay = class {
      constructor() {
        this.listening = false, this.inputEvent = l.None, this.inputEventListener = i.Disposable.None, this.emitter = new E({ onDidAddFirstListener: () => {
          this.listening = true, this.inputEventListener = this.inputEvent(this.emitter.fire, this.emitter);
        }, onDidRemoveLastListener: () => {
          this.listening = false, this.inputEventListener.dispose();
        } }), this.event = this.emitter.event;
      }
      set input(e3) {
        this.inputEvent = e3, this.listening && (this.inputEventListener.dispose(), this.inputEventListener = e3(this.emitter.fire, this.emitter));
      }
      dispose() {
        this.inputEventListener.dispose(), this.emitter.dispose();
      }
    }, t2.ValueWithChangeEvent = class {
      static const(e3) {
        return new A(e3);
      }
      constructor(e3) {
        this._value = e3, this._onDidChange = new E(), this.onDidChange = this._onDidChange.event;
      }
      get value() {
        return this._value;
      }
      set value(e3) {
        e3 !== this._value && (this._value = e3, this._onDidChange.fire(void 0));
      }
    };
    class A {
      constructor(e3) {
        this.value = e3, this.onDidChange = l.None;
      }
    }
  }, 355: (e2, t2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.createSingleCallFunction = function(e3, t3) {
      const s2 = this;
      let r2, n = false;
      return function() {
        if (n) return r2;
        if (n = true, t3) try {
          r2 = e3.apply(s2, arguments);
        } finally {
          t3();
        }
        else r2 = e3.apply(s2, arguments);
        return r2;
      };
    };
  }, 956: (e2, t2) => {
    var s2;
    Object.defineProperty(t2, "__esModule", { value: true }), t2.Iterable = void 0, (function(e3) {
      function t3(e4) {
        return e4 && "object" == typeof e4 && "function" == typeof e4[Symbol.iterator];
      }
      e3.is = t3;
      const s3 = Object.freeze([]);
      function* r2(e4) {
        yield e4;
      }
      e3.empty = function() {
        return s3;
      }, e3.single = r2, e3.wrap = function(e4) {
        return t3(e4) ? e4 : r2(e4);
      }, e3.from = function(e4) {
        return e4 || s3;
      }, e3.reverse = function* (e4) {
        for (let t4 = e4.length - 1; t4 >= 0; t4--) yield e4[t4];
      }, e3.isEmpty = function(e4) {
        return !e4 || true === e4[Symbol.iterator]().next().done;
      }, e3.first = function(e4) {
        return e4[Symbol.iterator]().next().value;
      }, e3.some = function(e4, t4) {
        let s4 = 0;
        for (const r3 of e4) if (t4(r3, s4++)) return true;
        return false;
      }, e3.find = function(e4, t4) {
        for (const s4 of e4) if (t4(s4)) return s4;
      }, e3.filter = function* (e4, t4) {
        for (const s4 of e4) t4(s4) && (yield s4);
      }, e3.map = function* (e4, t4) {
        let s4 = 0;
        for (const r3 of e4) yield t4(r3, s4++);
      }, e3.flatMap = function* (e4, t4) {
        let s4 = 0;
        for (const r3 of e4) yield* t4(r3, s4++);
      }, e3.concat = function* (...e4) {
        for (const t4 of e4) yield* t4;
      }, e3.reduce = function(e4, t4, s4) {
        let r3 = s4;
        for (const s5 of e4) r3 = t4(r3, s5);
        return r3;
      }, e3.slice = function* (e4, t4, s4 = e4.length) {
        for (t4 < 0 && (t4 += e4.length), s4 < 0 ? s4 += e4.length : s4 > e4.length && (s4 = e4.length); t4 < s4; t4++) yield e4[t4];
      }, e3.consume = function(t4, s4 = Number.POSITIVE_INFINITY) {
        const r3 = [];
        if (0 === s4) return [r3, t4];
        const n = t4[Symbol.iterator]();
        for (let t5 = 0; t5 < s4; t5++) {
          const t6 = n.next();
          if (t6.done) return [r3, e3.empty()];
          r3.push(t6.value);
        }
        return [r3, { [Symbol.iterator]: () => n }];
      }, e3.asyncToArray = async function(e4) {
        const t4 = [];
        for await (const s4 of e4) t4.push(s4);
        return Promise.resolve(t4);
      };
    })(s2 || (t2.Iterable = s2 = {}));
  }, 540: (e2, t2, s2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.DisposableMap = t2.ImmortalReference = t2.AsyncReferenceCollection = t2.ReferenceCollection = t2.SafeDisposable = t2.RefCountedDisposable = t2.MandatoryMutableDisposable = t2.MutableDisposable = t2.Disposable = t2.DisposableStore = t2.DisposableTracker = void 0, t2.setDisposableTracker = function(e3) {
      l = e3;
    }, t2.trackDisposable = u, t2.markAsDisposed = h, t2.markAsSingleton = function(e3) {
      return l?.markAsSingleton(e3), e3;
    }, t2.isDisposable = f, t2.dispose = p, t2.disposeIfDisposable = function(e3) {
      for (const t3 of e3) f(t3) && t3.dispose();
      return [];
    }, t2.combinedDisposable = function(...e3) {
      const t3 = _((() => p(e3)));
      return (function(e4, t4) {
        if (l) for (const s3 of e4) l.setParent(s3, t4);
      })(e3, t3), t3;
    }, t2.toDisposable = _, t2.disposeOnReturn = function(e3) {
      const t3 = new v();
      try {
        e3(t3);
      } finally {
        t3.dispose();
      }
    };
    const r2 = s2(732), n = s2(33), i = s2(714), o = s2(355), a = s2(956);
    let l = null;
    class c {
      constructor() {
        this.livingDisposables = /* @__PURE__ */ new Map();
      }
      static {
        this.idx = 0;
      }
      getDisposableData(e3) {
        let t3 = this.livingDisposables.get(e3);
        return t3 || (t3 = { parent: null, source: null, isSingleton: false, value: e3, idx: c.idx++ }, this.livingDisposables.set(e3, t3)), t3;
      }
      trackDisposable(e3) {
        const t3 = this.getDisposableData(e3);
        t3.source || (t3.source = new Error().stack);
      }
      setParent(e3, t3) {
        this.getDisposableData(e3).parent = t3;
      }
      markAsDisposed(e3) {
        this.livingDisposables.delete(e3);
      }
      markAsSingleton(e3) {
        this.getDisposableData(e3).isSingleton = true;
      }
      getRootParent(e3, t3) {
        const s3 = t3.get(e3);
        if (s3) return s3;
        const r3 = e3.parent ? this.getRootParent(this.getDisposableData(e3.parent), t3) : e3;
        return t3.set(e3, r3), r3;
      }
      getTrackedDisposables() {
        const e3 = /* @__PURE__ */ new Map();
        return [...this.livingDisposables.entries()].filter((([, t3]) => null !== t3.source && !this.getRootParent(t3, e3).isSingleton)).flatMap((([e4]) => e4));
      }
      computeLeakingDisposables(e3 = 10, t3) {
        let s3;
        if (t3) s3 = t3;
        else {
          const e4 = /* @__PURE__ */ new Map(), t4 = [...this.livingDisposables.values()].filter(((t5) => null !== t5.source && !this.getRootParent(t5, e4).isSingleton));
          if (0 === t4.length) return;
          const r3 = new Set(t4.map(((e5) => e5.value)));
          if (s3 = t4.filter(((e5) => !(e5.parent && r3.has(e5.parent)))), 0 === s3.length) throw new Error("There are cyclic diposable chains!");
        }
        if (!s3) return;
        function o2(e4) {
          const t4 = e4.source.split("\n").map(((e5) => e5.trim().replace("at ", ""))).filter(((e5) => "" !== e5));
          return (function(e5, t5) {
            for (; e5.length > 0 && t5.some(((t6) => "string" == typeof t6 ? t6 === e5[0] : e5[0].match(t6))); ) e5.shift();
          })(t4, ["Error", /^trackDisposable \(.*\)$/, /^DisposableTracker.trackDisposable \(.*\)$/]), t4.reverse();
        }
        const a2 = new i.SetMap();
        for (const e4 of s3) {
          const t4 = o2(e4);
          for (let s4 = 0; s4 <= t4.length; s4++) a2.add(t4.slice(0, s4).join("\n"), e4);
        }
        s3.sort((0, r2.compareBy)(((e4) => e4.idx), r2.numberComparator));
        let l2 = "", c2 = 0;
        for (const t4 of s3.slice(0, e3)) {
          c2++;
          const e4 = o2(t4), r3 = [];
          for (let t5 = 0; t5 < e4.length; t5++) {
            let i2 = e4[t5];
            i2 = `(shared with ${a2.get(e4.slice(0, t5 + 1).join("\n")).size}/${s3.length} leaks) at ${i2}`;
            const l3 = a2.get(e4.slice(0, t5).join("\n")), c3 = (0, n.groupBy)([...l3].map(((e5) => o2(e5)[t5])), ((e5) => e5));
            delete c3[e4[t5]];
            for (const [e5, t6] of Object.entries(c3)) r3.unshift(`    - stacktraces of ${t6.length} other leaks continue with ${e5}`);
            r3.unshift(i2);
          }
          l2 += `


==================== Leaking disposable ${c2}/${s3.length}: ${t4.value.constructor.name} ====================
${r3.join("\n")}
============================================================

`;
        }
        return s3.length > e3 && (l2 += `


... and ${s3.length - e3} more leaking disposables

`), { leaks: s3, details: l2 };
      }
    }
    function u(e3) {
      return l?.trackDisposable(e3), e3;
    }
    function h(e3) {
      l?.markAsDisposed(e3);
    }
    function d(e3, t3) {
      l?.setParent(e3, t3);
    }
    function f(e3) {
      return "object" == typeof e3 && null !== e3 && "function" == typeof e3.dispose && 0 === e3.dispose.length;
    }
    function p(e3) {
      if (a.Iterable.is(e3)) {
        const t3 = [];
        for (const s3 of e3) if (s3) try {
          s3.dispose();
        } catch (e4) {
          t3.push(e4);
        }
        if (1 === t3.length) throw t3[0];
        if (t3.length > 1) throw new AggregateError(t3, "Encountered errors while disposing of store");
        return Array.isArray(e3) ? [] : e3;
      }
      if (e3) return e3.dispose(), e3;
    }
    function _(e3) {
      const t3 = u({ dispose: (0, o.createSingleCallFunction)((() => {
        h(t3), e3();
      })) });
      return t3;
    }
    t2.DisposableTracker = c;
    class v {
      static {
        this.DISABLE_DISPOSED_WARNING = false;
      }
      constructor() {
        this._toDispose = /* @__PURE__ */ new Set(), this._isDisposed = false, u(this);
      }
      dispose() {
        this._isDisposed || (h(this), this._isDisposed = true, this.clear());
      }
      get isDisposed() {
        return this._isDisposed;
      }
      clear() {
        if (0 !== this._toDispose.size) try {
          p(this._toDispose);
        } finally {
          this._toDispose.clear();
        }
      }
      add(e3) {
        if (!e3) return e3;
        if (e3 === this) throw new Error("Cannot register a disposable on itself!");
        return d(e3, this), this._isDisposed ? v.DISABLE_DISPOSED_WARNING || void 0 : this._toDispose.add(e3), e3;
      }
      delete(e3) {
        if (e3) {
          if (e3 === this) throw new Error("Cannot dispose a disposable on itself!");
          this._toDispose.delete(e3), e3.dispose();
        }
      }
      deleteAndLeak(e3) {
        e3 && this._toDispose.has(e3) && (this._toDispose.delete(e3), d(e3, null));
      }
    }
    t2.DisposableStore = v;
    class E {
      static {
        this.None = Object.freeze({ dispose() {
        } });
      }
      constructor() {
        this._store = new v(), u(this), d(this._store, this);
      }
      dispose() {
        h(this), this._store.dispose();
      }
      _register(e3) {
        if (e3 === this) throw new Error("Cannot register a disposable on itself!");
        return this._store.add(e3);
      }
    }
    t2.Disposable = E;
    class g {
      constructor() {
        this._isDisposed = false, u(this);
      }
      get value() {
        return this._isDisposed ? void 0 : this._value;
      }
      set value(e3) {
        this._isDisposed || e3 === this._value || (this._value?.dispose(), e3 && d(e3, this), this._value = e3);
      }
      clear() {
        this.value = void 0;
      }
      dispose() {
        this._isDisposed = true, h(this), this._value?.dispose(), this._value = void 0;
      }
      clearAndLeak() {
        const e3 = this._value;
        return this._value = void 0, e3 && d(e3, null), e3;
      }
    }
    t2.MutableDisposable = g, t2.MandatoryMutableDisposable = class {
      constructor(e3) {
        this._disposable = new g(), this._isDisposed = false, this._disposable.value = e3;
      }
      get value() {
        return this._disposable.value;
      }
      set value(e3) {
        this._isDisposed || e3 === this._disposable.value || (this._disposable.value = e3);
      }
      dispose() {
        this._isDisposed = true, this._disposable.dispose();
      }
    }, t2.RefCountedDisposable = class {
      constructor(e3) {
        this._disposable = e3, this._counter = 1;
      }
      acquire() {
        return this._counter++, this;
      }
      release() {
        return 0 == --this._counter && this._disposable.dispose(), this;
      }
    }, t2.SafeDisposable = class {
      constructor() {
        this.dispose = () => {
        }, this.unset = () => {
        }, this.isset = () => false, u(this);
      }
      set(e3) {
        let t3 = e3;
        return this.unset = () => t3 = void 0, this.isset = () => void 0 !== t3, this.dispose = () => {
          t3 && (t3(), t3 = void 0, h(this));
        }, this;
      }
    }, t2.ReferenceCollection = class {
      constructor() {
        this.references = /* @__PURE__ */ new Map();
      }
      acquire(e3, ...t3) {
        let s3 = this.references.get(e3);
        s3 || (s3 = { counter: 0, object: this.createReferencedObject(e3, ...t3) }, this.references.set(e3, s3));
        const { object: r3 } = s3, n2 = (0, o.createSingleCallFunction)((() => {
          0 == --s3.counter && (this.destroyReferencedObject(e3, s3.object), this.references.delete(e3));
        }));
        return s3.counter++, { object: r3, dispose: n2 };
      }
    }, t2.AsyncReferenceCollection = class {
      constructor(e3) {
        this.referenceCollection = e3;
      }
      async acquire(e3, ...t3) {
        const s3 = this.referenceCollection.acquire(e3, ...t3);
        try {
          return { object: await s3.object, dispose: () => s3.dispose() };
        } catch (e4) {
          throw s3.dispose(), e4;
        }
      }
    }, t2.ImmortalReference = class {
      constructor(e3) {
        this.object = e3;
      }
      dispose() {
      }
    };
    class m {
      constructor() {
        this._store = /* @__PURE__ */ new Map(), this._isDisposed = false, u(this);
      }
      dispose() {
        h(this), this._isDisposed = true, this.clearAndDisposeAll();
      }
      clearAndDisposeAll() {
        if (this._store.size) try {
          p(this._store.values());
        } finally {
          this._store.clear();
        }
      }
      has(e3) {
        return this._store.has(e3);
      }
      get size() {
        return this._store.size;
      }
      get(e3) {
        return this._store.get(e3);
      }
      set(e3, t3, s3 = false) {
        this._isDisposed && void 0, s3 || this._store.get(e3)?.dispose(), this._store.set(e3, t3);
      }
      deleteAndDispose(e3) {
        this._store.get(e3)?.dispose(), this._store.delete(e3);
      }
      deleteAndLeak(e3) {
        const t3 = this._store.get(e3);
        return this._store.delete(e3), t3;
      }
      keys() {
        return this._store.keys();
      }
      values() {
        return this._store.values();
      }
      [Symbol.iterator]() {
        return this._store[Symbol.iterator]();
      }
    }
    t2.DisposableMap = m;
  }, 711: (e2, t2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.LinkedList = void 0;
    class s2 {
      static {
        this.Undefined = new s2(void 0);
      }
      constructor(e3) {
        this.element = e3, this.next = s2.Undefined, this.prev = s2.Undefined;
      }
    }
    class r2 {
      constructor() {
        this._first = s2.Undefined, this._last = s2.Undefined, this._size = 0;
      }
      get size() {
        return this._size;
      }
      isEmpty() {
        return this._first === s2.Undefined;
      }
      clear() {
        let e3 = this._first;
        for (; e3 !== s2.Undefined; ) {
          const t3 = e3.next;
          e3.prev = s2.Undefined, e3.next = s2.Undefined, e3 = t3;
        }
        this._first = s2.Undefined, this._last = s2.Undefined, this._size = 0;
      }
      unshift(e3) {
        return this._insert(e3, false);
      }
      push(e3) {
        return this._insert(e3, true);
      }
      _insert(e3, t3) {
        const r3 = new s2(e3);
        if (this._first === s2.Undefined) this._first = r3, this._last = r3;
        else if (t3) {
          const e4 = this._last;
          this._last = r3, r3.prev = e4, e4.next = r3;
        } else {
          const e4 = this._first;
          this._first = r3, r3.next = e4, e4.prev = r3;
        }
        this._size += 1;
        let n = false;
        return () => {
          n || (n = true, this._remove(r3));
        };
      }
      shift() {
        if (this._first !== s2.Undefined) {
          const e3 = this._first.element;
          return this._remove(this._first), e3;
        }
      }
      pop() {
        if (this._last !== s2.Undefined) {
          const e3 = this._last.element;
          return this._remove(this._last), e3;
        }
      }
      _remove(e3) {
        if (e3.prev !== s2.Undefined && e3.next !== s2.Undefined) {
          const t3 = e3.prev;
          t3.next = e3.next, e3.next.prev = t3;
        } else e3.prev === s2.Undefined && e3.next === s2.Undefined ? (this._first = s2.Undefined, this._last = s2.Undefined) : e3.next === s2.Undefined ? (this._last = this._last.prev, this._last.next = s2.Undefined) : e3.prev === s2.Undefined && (this._first = this._first.next, this._first.prev = s2.Undefined);
        this._size -= 1;
      }
      *[Symbol.iterator]() {
        let e3 = this._first;
        for (; e3 !== s2.Undefined; ) yield e3.element, e3 = e3.next;
      }
    }
    t2.LinkedList = r2;
  }, 714: (e2, t2) => {
    var s2;
    Object.defineProperty(t2, "__esModule", { value: true }), t2.SetMap = t2.BidirectionalMap = t2.CounterSet = t2.Touch = void 0, t2.getOrSet = function(e3, t3, s3) {
      let r2 = e3.get(t3);
      return void 0 === r2 && (r2 = s3, e3.set(t3, r2)), r2;
    }, t2.mapToString = function(e3) {
      const t3 = [];
      return e3.forEach(((e4, s3) => {
        t3.push(`${s3} => ${e4}`);
      })), `Map(${e3.size}) {${t3.join(", ")}}`;
    }, t2.setToString = function(e3) {
      const t3 = [];
      return e3.forEach(((e4) => {
        t3.push(e4);
      })), `Set(${e3.size}) {${t3.join(", ")}}`;
    }, t2.mapsStrictEqualIgnoreOrder = function(e3, t3) {
      if (e3 === t3) return true;
      if (e3.size !== t3.size) return false;
      for (const [s3, r2] of e3) if (!t3.has(s3) || t3.get(s3) !== r2) return false;
      for (const [s3] of t3) if (!e3.has(s3)) return false;
      return true;
    }, (function(e3) {
      e3[e3.None = 0] = "None", e3[e3.AsOld = 1] = "AsOld", e3[e3.AsNew = 2] = "AsNew";
    })(s2 || (t2.Touch = s2 = {})), t2.CounterSet = class {
      constructor() {
        this.map = /* @__PURE__ */ new Map();
      }
      add(e3) {
        return this.map.set(e3, (this.map.get(e3) || 0) + 1), this;
      }
      delete(e3) {
        let t3 = this.map.get(e3) || 0;
        return 0 !== t3 && (t3--, 0 === t3 ? this.map.delete(e3) : this.map.set(e3, t3), true);
      }
      has(e3) {
        return this.map.has(e3);
      }
    }, t2.BidirectionalMap = class {
      constructor(e3) {
        if (this._m1 = /* @__PURE__ */ new Map(), this._m2 = /* @__PURE__ */ new Map(), e3) for (const [t3, s3] of e3) this.set(t3, s3);
      }
      clear() {
        this._m1.clear(), this._m2.clear();
      }
      set(e3, t3) {
        this._m1.set(e3, t3), this._m2.set(t3, e3);
      }
      get(e3) {
        return this._m1.get(e3);
      }
      getKey(e3) {
        return this._m2.get(e3);
      }
      delete(e3) {
        const t3 = this._m1.get(e3);
        return void 0 !== t3 && (this._m1.delete(e3), this._m2.delete(t3), true);
      }
      forEach(e3, t3) {
        this._m1.forEach(((s3, r2) => {
          e3.call(t3, s3, r2, this);
        }));
      }
      keys() {
        return this._m1.keys();
      }
      values() {
        return this._m1.values();
      }
    }, t2.SetMap = class {
      constructor() {
        this.map = /* @__PURE__ */ new Map();
      }
      add(e3, t3) {
        let s3 = this.map.get(e3);
        s3 || (s3 = /* @__PURE__ */ new Set(), this.map.set(e3, s3)), s3.add(t3);
      }
      delete(e3, t3) {
        const s3 = this.map.get(e3);
        s3 && (s3.delete(t3), 0 === s3.size && this.map.delete(e3));
      }
      forEach(e3, t3) {
        const s3 = this.map.get(e3);
        s3 && s3.forEach(t3);
      }
      get(e3) {
        return this.map.get(e3) || /* @__PURE__ */ new Set();
      }
    };
  }, 79: (e2, t2) => {
    Object.defineProperty(t2, "__esModule", { value: true }), t2.StopWatch = void 0;
    const s2 = globalThis.performance && "function" == typeof globalThis.performance.now;
    class r2 {
      static create(e3) {
        return new r2(e3);
      }
      constructor(e3) {
        this._now = s2 && false === e3 ? Date.now : globalThis.performance.now.bind(globalThis.performance), this._startTime = this._now(), this._stopTime = -1;
      }
      stop() {
        this._stopTime = this._now();
      }
      reset() {
        this._startTime = this._now(), this._stopTime = -1;
      }
      elapsed() {
        return -1 !== this._stopTime ? this._stopTime - this._startTime : this._now() - this._startTime;
      }
    }
    t2.StopWatch = r2;
  } }, t = {};
  function s(r2) {
    var n = t[r2];
    if (void 0 !== n) return n.exports;
    var i = t[r2] = { exports: {} };
    return e[r2](i, i.exports, s), i.exports;
  }
  var r = {};
  return (() => {
    var e2 = r;
    Object.defineProperty(e2, "__esModule", { value: true }), e2.UnicodeGraphemesAddon = void 0;
    const t2 = s(106);
    e2.UnicodeGraphemesAddon = class {
      constructor() {
        this._oldVersion = "";
      }
      activate(e3) {
        this._provider15 || (this._provider15 = new t2.UnicodeGraphemeProvider(false)), this._provider15Graphemes || (this._provider15Graphemes = new t2.UnicodeGraphemeProvider(true));
        const s2 = e3.unicode;
        this._unicode = s2, s2.register(this._provider15), s2.register(this._provider15Graphemes), this._oldVersion = s2.activeVersion, s2.activeVersion = "15-graphemes";
      }
      dispose() {
        this._unicode && (this._unicode.activeVersion = this._oldVersion);
      }
    };
  })(), r;
})()));
