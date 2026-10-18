/* ==========================================================================
   charts.js — Canvas/SVG 图表引擎（零依赖）
   ========================================================================== */

const Charts = (() => {

  const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const T = (name) => getComputedStyle(document.body).getPropertyValue(name).trim() || css(name);

  function setup(canvas) {
    const dpr = window.devicePixelRatio || 1;
    const r = canvas.getBoundingClientRect();
    canvas.width = Math.max(1, r.width * dpr);
    canvas.height = Math.max(1, r.height * dpr);
    const ctx = canvas.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    return { ctx, w: r.width, h: r.height };
  }

  function fmt(n, d = 2) {
    return n.toLocaleString('en-US', { minimumFractionDigits: d, maximumFractionDigits: d });
  }
  function pct(n, d = 2) { return (n >= 0 ? '+' : '') + n.toFixed(d) + '%'; }

  /* ---------- 9 档涨跌色阶（§4.2） ---------- */
  const LEVELS = [8, 6, 4, 2, 0.5];   // 档位阈值（%）
  function tileColor(chg) {
    const c = T('--up-deep'), cm = T('--up-mid'), cb = T('--up-bright');
    const d = T('--down-deep'), dm = T('--down-mid'), db = T('--down-bright');
    if (chg >= 8) return cb;
    if (chg >= 6) return cb;
    if (chg >= 4) return cb;
    if (chg >= 2) return cm;
    if (chg >= 0.5) return cm;
    if (chg > -0.5) return '#2A2C31';
    if (chg > -2) return dm;
    if (chg > -4) return dm;
    if (chg > -6) return dm;
    if (chg > -8) return d;
    return d;
  }
  /* 色阶：从深档插值到亮档（实测 NVDA -0.63% ≈ #781942 深档、META +3.84% ≈ #0DB397 亮档） */
  function tileFill(chg) {
    const k = Math.min(1, Math.abs(chg) / 6);   // 0..1，6% 以上封顶为最亮
    if (Math.abs(chg) < 0.3) return '#2A2C31';  // 平盘灰
    const from = hex2rgb(chg >= 0 ? T('--up-deep') : T('--down-deep'));
    const to = hex2rgb(chg >= 0 ? T('--up-bright') : T('--down-bright'));
    const col = mix(from, to, k);
    return `rgb(${col[0]},${col[1]},${col[2]})`;
  }
  function hex2rgb(h) { const n = parseInt(h.replace('#', ''), 16); return [(n >> 16) & 255, (n >> 8) & 255, n & 255]; }
  function mix(a, b, k) { return a.map((v, i) => Math.round(v + (b[i] - v) * k)); }

  /* ---------- Squarified Treemap（§6.1 核心算法） ---------- */
  function squarify(data, x, y, w, h) {
    // data: [{name, size(面积权重), chg, ...}] 已按 size 降序
    const total = data.reduce((s, d) => s + d.size, 0);
    const scale = (w * h) / total;
    const rects = [];
    const sorted = [...data].sort((a, b) => b.size - a.size);
    layout(sorted.map(d => ({ ...d, _area: d.size * scale })), x, y, w, h);
    return rects;

    function worst(row, len) {
      if (!row.length) return Infinity;
      const s = row.reduce((t, r) => t + r._area, 0);
      const mx = Math.max(...row.map(r => r._area));
      const mn = Math.min(...row.map(r => r._area));
      const l2 = len * len;
      return Math.max((l2 * mx) / (s * s), (s * s) / (l2 * mn));
    }
    function layoutrow(row, x, y, w, h) {
      const s = row.reduce((t, r) => t + r._area, 0);
      if (w >= h) { const rw = s / h; let ry = y;
        row.forEach(r => { const rh = r._area / rw; rects.push({ ...r, x, y: ry, w: rw, h: rh }); ry += rh; });
        return { x: x + rw, y, w: w - rw, h };
      } else { const rh = s / w; let rx = x;
        row.forEach(r => { const rw = r._area / rh; rects.push({ ...r, x: rx, y, w: rw, h: rh }); rx += rw; });
        return { x, y: y + rh, w, h: h - rh };
      }
    }
    function layout(items, x, y, w, h) {
      let row = [];
      while (items.length) {
        const it = items[0];
        const len = Math.min(w, h);
        if (row.length && worst(row, len) < worst([...row, it], len)) {
          ({ x, y, w, h } = layoutrow(row, x, y, w, h));
          row = [];
        }
        row.push(it);
        items.shift();
      }
      if (row.length) layoutrow(row, x, y, w, h);
    }
  }

  function drawTreemap(canvas, groups, opts = {}) {
    const { ctx, w, h } = setup(canvas);
    const GAP = 2;
    ctx.clearRect(0, 0, w, h);
    const tiles = [];
    // 外层：按组分配面积（组 size = 成员之和）
    const gdata = groups.map(g => ({ ...g, size: g.items.reduce((s, i) => s + i.size, 0) }));
    const grects = squarify(gdata, 0, 0, w, h);
    for (const g of grects) {
      const labelH = 16;
      const inner = { x: g.x + GAP, y: g.y + GAP + labelH, w: g.w - GAP * 2, h: g.h - GAP * 2 - labelH };
      if (inner.w <= 4 || inner.h <= 4) continue;
      // 组标签条
      ctx.fillStyle = 'rgba(255,255,255,0.05)';
      rr(ctx, g.x + GAP, g.y + GAP, g.w - GAP * 2, labelH, 3);
      ctx.fill();
      ctx.fillStyle = T('--text-secondary');
      ctx.font = `600 9px ${T('--font-sans') || 'Inter, sans-serif'}`;
      ctx.textBaseline = 'middle';
      ctx.fillText(g.name, g.x + GAP + 6, g.y + GAP + labelH / 2);
      // 内层 treemap
      const nodes = squarify(g.items, inner.x, inner.y, inner.w, inner.h);
      for (const n of nodes) {
        if (n.w < 4 || n.h < 4) continue;
        ctx.fillStyle = tileFill(n.chg);
        rr(ctx, n.x + 1, n.y + 1, n.w - 2, n.h - 2, 2);
        ctx.fill();
        tiles.push({ ...n, x: n.x + 1, y: n.y + 1, w: n.w - 2, h: n.h - 2, group: g.name });
        // 文字
        if (n.w > 46 && n.h > 26) {
          const cx = n.x + n.w / 2, cy = n.y + n.h / 2;
          ctx.textAlign = 'center';
          if (n.w > 60 && n.h > 40) {
            // logo 圆
            ctx.beginPath(); ctx.arc(cx, cy - 12, 9, 0, Math.PI * 2);
            ctx.fillStyle = n.logoBg || '#333'; ctx.fill();
            ctx.fillStyle = '#fff'; ctx.font = `700 8px ${T('--font-sans') || 'sans-serif'}`;
            ctx.fillText(n.name.slice(0, 2), cx, cy - 12);
            ctx.font = `700 10px #fff`;
            ctx.fillText(n.name, cx, cy + 3);
            ctx.font = `600 9px rgba(255,255,255,.85)`;
            ctx.fillText(pct(n.chg), cx, cy + 14);
          } else {
            ctx.fillStyle = '#fff'; ctx.font = `700 9px ${T('--font-sans') || 'sans-serif'}`;
            ctx.fillText(n.name, cx, cy - 5);
            ctx.font = `600 8px rgba(255,255,255,.85)`;
            ctx.fillText(pct(n.chg), cx, cy + 5);
          }
          ctx.textAlign = 'left';
        }
      }
    }
    canvas.__tiles = tiles;
    return tiles;
  }

  function rr(ctx, x, y, w, h, r) {
    r = Math.min(r, w / 2, h / 2);
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
  }

  /* ---------- Sparkline（指数卡分时线，带渐变填充） ---------- */
  function drawSpark(canvas, data, opts = {}) {
    const { ctx, w, h } = setup(canvas);
    const min = Math.min(...data), max = Math.max(...data);
    const pad = 2;
    const X = i => pad + (i / (data.length - 1)) * (w - pad * 2);
    const Y = v => pad + (1 - (v - min) / (max - min || 1)) * (h - pad * 2);
    const chg = data[data.length - 1] - data[0];
    const col = opts.color || (chg >= 0 ? T('--up-btn') : T('--down-btn'));
    ctx.clearRect(0, 0, w, h);
    // 渐变填充
    const grad = ctx.createLinearGradient(0, 0, 0, h);
    grad.addColorStop(0, col + '40');
    grad.addColorStop(1, col + '00');
    ctx.beginPath();
    ctx.moveTo(X(0), Y(data[0]));
    data.forEach((v, i) => ctx.lineTo(X(i), Y(v)));
    ctx.lineTo(X(data.length - 1), h); ctx.lineTo(X(0), h); ctx.closePath();
    ctx.fillStyle = grad; ctx.fill();
    // 线
    ctx.beginPath();
    data.forEach((v, i) => i ? ctx.lineTo(X(i), Y(v)) : ctx.moveTo(X(i), Y(v)));
    ctx.strokeStyle = col; ctx.lineWidth = 1.3; ctx.stroke();
  }

  /* ---------- 迷你K线（自选宫格） ---------- */
  function drawMiniK(canvas, candles) {
    const { ctx, w, h } = setup(canvas);
    ctx.clearRect(0, 0, w, h);
    const n = candles.length;
    const lo = Math.min(...candles.map(c => c.l));
    const hi = Math.max(...candles.map(c => c.h));
    const pad = 4;
    const bw = (w - pad * 2) / n;
    const bodyW = Math.max(2, bw * 0.62);
    const Y = v => pad + (1 - (v - lo) / (hi - lo || 1)) * (h - pad * 2);
    candles.forEach((c, i) => {
      const x = pad + i * bw + bw / 2;
      const up = c.c >= c.o;
      ctx.strokeStyle = up ? T('--up-btn') : T('--down-btn');
      ctx.lineWidth = 1;
      ctx.beginPath(); ctx.moveTo(x, Y(c.h)); ctx.lineTo(x, Y(c.l)); ctx.stroke();
      ctx.fillStyle = up ? T('--up-btn') : T('--down-btn');
      const y1 = Y(Math.max(c.o, c.c)), y2 = Y(Math.min(c.o, c.c));
      ctx.fillRect(x - bodyW / 2, y1, bodyW, Math.max(1, y2 - y1));
    });
    // 最新价水平虚线
    const last = candles[candles.length - 1];
    const y = Y(last.c);
    ctx.setLineDash([3, 3]);
    ctx.strokeStyle = T('--price-line'); ctx.lineWidth = 1;
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
    ctx.setLineDash([]);
  }

  /* ---------- 主 K 线（Canvas，含十字光标 + 最新价标签 + 成交量） ---------- */
  class CandleChart {
    constructor(canvas, candles, opts = {}) {
      this.canvas = canvas;
      this.candles = candles;
      this.opts = opts;
      this.cursor = null;
      this.visible = [Math.max(0, candles.length - 80), candles.length];
      canvas.addEventListener('mousemove', e => {
        const r = canvas.getBoundingClientRect();
        this.cursor = { x: e.clientX - r.left, y: e.clientY - r.top };
        this.draw();
        if (opts.onCursor) opts.onCursor(this.cursorCandle());
      });
      canvas.addEventListener('mouseleave', () => { this.cursor = null; this.draw(); if (opts.onCursor) opts.onCursor(null); });
      canvas.addEventListener('wheel', e => {
        e.preventDefault();
        let [a, b] = this.visible;
        const span = b - a, mid = (a + b) / 2;
        const ns = Math.max(20, Math.min(candles.length, span * (e.deltaY > 0 ? 1.12 : 0.9)));
        a = Math.max(0, Math.min(candles.length - ns, Math.round(mid - ns / 2)));
        this.visible = [a, Math.round(a + ns)];
        this.draw();
      }, { passive: false });
      this.draw();
    }
    cursorCandle() {
      if (!this.cursor) return null;
      const [a, b] = this.visible;
      const i = a + Math.round((this.cursor.x - 44) / this.bw - 0.5);
      return this.candles[Math.max(0, Math.min(b - 1, i))] || null;
    }
    draw() {
      const { ctx, w, h } = setup(this.canvas);
      const { candles } = this;
      const [a, b] = this.visible;
      const slice = candles.slice(a, b);
      if (!slice.length) return;
      const padL = 44, padR = 56, padT = 8;
      const volH = Math.round(h * 0.16);
      const priceH = h - volH - padT - 4;
      const plotW = w - padL - padR;
      const n = slice.length;
      const bw = plotW / n;
      this.bw = bw;
      const lo = Math.min(...slice.map(c => c.l));
      const hi = Math.max(...slice.map(c => c.h));
      const Y = v => padT + (1 - (v - lo) / (hi - lo || 1)) * (priceH - 8);
      const volMax = Math.max(...slice.map(c => c.v));
      const volY = v => h - (v / volMax) * (volH - 6);

      ctx.clearRect(0, 0, w, h);
      // 网格 + 价格标签
      ctx.font = `10px ${T('--font-mono') || 'monospace'}`;
      ctx.textBaseline = 'middle';
      for (let i = 0; i <= 5; i++) {
        const v = lo + (hi - lo) * (i / 5);
        const y = Y(v);
        ctx.strokeStyle = 'rgba(255,255,255,0.05)';
        ctx.setLineDash([2, 3]);
        ctx.beginPath(); ctx.moveTo(padL, y); ctx.lineTo(w - padR, y); ctx.stroke();
        ctx.setLineDash([]);
        ctx.fillStyle = T('--text-tertiary');
        ctx.fillText(fmt(v, 2), w - padR + 6, y);
      }
      // 蜡烛
      slice.forEach((c, i) => {
        const x = padL + i * bw + bw / 2;
        const up = c.c >= c.o;
        const col = up ? T('--up-btn') : T('--down-btn');
        ctx.strokeStyle = col; ctx.lineWidth = 1;
        ctx.beginPath(); ctx.moveTo(x, Y(c.h)); ctx.lineTo(x, Y(c.l)); ctx.stroke();
        ctx.fillStyle = col;
        const y1 = Y(Math.max(c.o, c.c)), y2 = Y(Math.min(c.o, c.c));
        ctx.fillRect(x - bw * 0.31, y1, bw * 0.62, Math.max(1, y2 - y1));
        // 成交量柱
        ctx.globalAlpha = .75;
        ctx.fillRect(x - bw * 0.31, volY(c.v), bw * 0.62, h - volY(c.v));
        ctx.globalAlpha = 1;
      });
      // 事件徽章（E/D）
      (this.opts.events || []).forEach(ev => {
        const idx = candles.indexOf(ev.candle);
        if (idx < a || idx >= b) return;
        const x = padL + (idx - a) * bw + bw / 2;
        const y = Y(ev.candle.h) - 12;
        ctx.beginPath(); ctx.arc(x, y, 6, 0, Math.PI * 2);
        ctx.fillStyle = ev.type === 'E' ? T('--up-btn') : T('--c-chart-line');
        ctx.fill();
        ctx.fillStyle = '#fff'; ctx.font = '700 7px Inter'; ctx.textAlign = 'center';
        ctx.fillText(ev.type, x, y); ctx.textAlign = 'left';
      });
      // 最新价虚线 + 右侧蓝底标签
      const last = candles[candles.length - 1];
      if (last) {
        const y = Y(last.c);
        ctx.setLineDash([3, 3]);
        ctx.strokeStyle = T('--price-line'); ctx.lineWidth = 1;
        ctx.beginPath(); ctx.moveTo(padL, y); ctx.lineTo(w - padR, y); ctx.stroke();
        ctx.setLineDash([]);
        const tw = ctx.measureText(fmt(last.c)).width + 8;
        ctx.fillStyle = T('--price-line');
        rr(ctx, w - padR + 1, y - 8, tw, 16, 2); ctx.fill();
        ctx.fillStyle = '#fff'; ctx.fillText(fmt(last.c), w - padR + 5, y);
      }
      // 十字光标
      if (this.cursor) {
        ctx.setLineDash([3, 3]);
        ctx.strokeStyle = 'rgba(255,255,255,.35)';
        ctx.beginPath(); ctx.moveTo(this.cursor.x, padT); ctx.lineTo(this.cursor.x, h); ctx.stroke();
        ctx.beginPath(); ctx.moveTo(padL, this.cursor.y); ctx.lineTo(w - padR, this.cursor.y); ctx.stroke();
        ctx.setLineDash([]);
        // 价格标签
        const v = lo + (1 - (this.cursor.y - padT) / (priceH - 8)) * (hi - lo);
        const t2 = fmt(v), tw2 = ctx.measureText(t2).width + 8;
        ctx.fillStyle = '#3A3D44';
        rr(ctx, w - padR + 1, this.cursor.y - 8, tw2, 16, 2); ctx.fill();
        ctx.fillStyle = '#fff'; ctx.fillText(t2, w - padR + 5, this.cursor.y);
      }
      // 时间轴标签
      ctx.fillStyle = T('--text-tertiary');
      for (let i = 0; i < n; i += Math.ceil(n / 6)) {
        ctx.fillText(slice[i].t, padL + i * bw, h - 3);
      }
    }
  }

  /* ---------- 桑基图（SVG，三层四节点） ---------- */
  function drawSankey(svg, data) {
    // data: { nodes:[{id,label,val,x,color}], links:[{s,t,val,color}] }
    const W = 1000, H = 300;
    svg.setAttribute('viewBox', `0 0 ${W} ${H}`);
    svg.innerHTML = '';
    const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');
    svg.appendChild(defs);
    const cols = [[], [], []];
    data.nodes.forEach(n => cols[n.x].push(n));
    const colH = [0, 0, 0];
    data.nodes.forEach(n => colH[n.x] += n.val);
    const GAPN = 30;
    // 全局统一 factor（取各列最小值），保证流带两端宽度一致
    const factor = Math.min(...cols.map((col, ci) => (H - 24 - (col.length - 1) * GAPN) / colH[ci]), 2.6);
    cols.forEach((col, ci) => {
      let y = (H - colH[ci] * factor - (col.length - 1) * GAPN) / 2;
      col.forEach(n => {
        n.h = n.val * factor;
        n.y = y;
        y += n.h + GAPN;
        n.sx = 40 + ci * 330;
        n.sw = 18;
      });
    });
    // links（贝塞尔流带）
    data.links.forEach((lk, i) => {
      const s = data.nodes.find(n => n.id === lk.s);
      const t = data.nodes.find(n => n.id === lk.t);
      s._used = s._used || 0; t._used = t._used || 0;
      const lh = lk.val * factor;
      const y1 = s.y + s._used + lh / 2, y2 = t.y + t._used + lh / 2;
      s._used += lh; t._used += lh;
      const x1 = s.sx + s.sw, x2 = t.sx;
      const cx = (x1 + x2) / 2;
      const grad = document.createElementNS('http://www.w3.org/2000/svg', 'linearGradient');
      grad.setAttribute('id', 'lg' + i);
      grad.setAttribute('x1', '0'); grad.setAttribute('y1', '0');
      grad.setAttribute('x2', '1'); grad.setAttribute('y2', '0');
      const st1 = document.createElementNS('http://www.w3.org/2000/svg', 'stop');
      st1.setAttribute('offset', '0'); st1.setAttribute('stop-color', lk.color); st1.setAttribute('stop-opacity', '.35');
      const st2 = document.createElementNS('http://www.w3.org/2000/svg', 'stop');
      st2.setAttribute('offset', '1'); st2.setAttribute('stop-color', lk.color); st2.setAttribute('stop-opacity', '.5');
      grad.appendChild(st1); grad.appendChild(st2);
      defs.appendChild(grad);
      const p = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      p.setAttribute('d', `M ${x1} ${y1 - lh / 2} C ${cx} ${y1 - lh / 2}, ${cx} ${y2 - lh / 2}, ${x2} ${y2 - lh / 2} L ${x2} ${y2 + lh / 2} C ${cx} ${y2 + lh / 2}, ${cx} ${y1 + lh / 2}, ${x1} ${y1 + lh / 2} Z`);
      p.setAttribute('fill', `url(#lg${i})`);
      svg.appendChild(p);
    });
    // nodes
    data.nodes.forEach(n => {
      const r = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
      r.setAttribute('x', n.sx); r.setAttribute('y', n.y);
      r.setAttribute('width', n.sw); r.setAttribute('height', n.h);
      r.setAttribute('rx', 3); r.setAttribute('fill', n.color);
      svg.appendChild(r);
      const tx = document.createElementNS('http://www.w3.org/2000/svg', 'text');
      tx.setAttribute('x', n.sx + n.sw + 8);
      tx.setAttribute('y', n.y + 10);
      tx.setAttribute('fill', '#9AA0A6');
      tx.setAttribute('font-size', '11');
      tx.textContent = n.label;
      svg.appendChild(tx);
      const tv = document.createElementNS('http://www.w3.org/2000/svg', 'text');
      tv.setAttribute('x', n.sx + n.sw + 8);
      tv.setAttribute('y', n.y + 26);
      tv.setAttribute('fill', '#E6E8EB');
      tv.setAttribute('font-size', '12');
      tv.setAttribute('font-weight', '700');
      tv.setAttribute('font-variant-numeric', 'tabular-nums');
      tv.textContent = '$' + n.val.toFixed(1) + 'B';
      svg.appendChild(tv);
      if (n.yoy) {
        const ty = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        ty.setAttribute('x', n.sx + n.sw + 8);
        ty.setAttribute('y', n.y + 42);
        ty.setAttribute('fill', n.yoy >= 0 ? T('--up-text') : T('--down-text'));
        ty.setAttribute('font-size', '11');
        ty.setAttribute('font-weight', '600');
        ty.textContent = (n.yoy >= 0 ? '↑ +' : '↓ ') + n.yoy.toFixed(1) + '%';
        svg.appendChild(ty);
      }
    });
  }

  /* ---------- 组合图（柱=收入，线=毛利率，双 Y 轴） ---------- */
  function drawCombo(canvas, data) {
    const { ctx, w, h } = setup(canvas);
    ctx.clearRect(0, 0, w, h);
    const padL = 46, padR = 46, padT = 16, padB = 24;
    const plotW = w - padL - padR, plotH = h - padT - padB;
    const maxBar = Math.max(...data.map(d => d.bar));
    const bw = plotW / data.length;
    const lineVals = data.map(d => d.line);
    const lLo = Math.floor(Math.min(...lineVals)) - 2, lHi = Math.ceil(Math.max(...lineVals)) + 2;
    const Y = v => padT + (1 - v / (maxBar * 1.15)) * plotH;
    const Yr = v => padT + (1 - (v - lLo) / (lHi - lLo)) * plotH;  // 毛利率右轴自适应
    ctx.font = `10px ${T('--font-mono') || 'monospace'}`;
    // 网格
    for (let i = 0; i <= 4; i++) {
      const y = padT + (i / 4) * plotH;
      ctx.strokeStyle = 'rgba(255,255,255,.05)';
      ctx.setLineDash([2, 3]); ctx.beginPath(); ctx.moveTo(padL, y); ctx.lineTo(w - padR, y); ctx.stroke(); ctx.setLineDash([]);
      ctx.fillStyle = T('--text-tertiary');
      ctx.textAlign = 'left';
      ctx.fillText('$' + fmt(maxBar * 1.15 * (1 - i / 4), 0) + 'B', 4, y + 3);
      ctx.textAlign = 'right';
      ctx.fillText((lHi - (lHi - lLo) * i / 4).toFixed(0) + '%', w - 4, y + 3);
    }
    ctx.textAlign = 'center';
    data.forEach((d, i) => {
      const x = padL + i * bw + bw / 2;
      // 柱
      ctx.fillStyle = T('--c-chart-bar');
      rr(ctx, x - bw * 0.24, Y(d.bar), bw * 0.48, padT + plotH - Y(d.bar), 2);
      ctx.fill();
      ctx.fillStyle = '#fff'; ctx.font = `600 9px Inter`;
      ctx.fillText('$' + fmt(d.bar, 2) + 'B', x, Y(d.bar) - 8);
      ctx.fillStyle = T('--text-tertiary'); ctx.font = `10px Inter`;
      ctx.fillText(d.x, x, h - 8);
    });
    // 线
    ctx.beginPath();
    data.forEach((d, i) => {
      const x = padL + i * bw + bw / 2;
      i ? ctx.lineTo(x, Yr(d.line)) : ctx.moveTo(x, Yr(d.line));
    });
    ctx.strokeStyle = T('--c-chart-line'); ctx.lineWidth = 2; ctx.stroke();
    data.forEach((d, i) => {
      const x = padL + i * bw + bw / 2;
      ctx.beginPath(); ctx.arc(x, Yr(d.line), 3, 0, Math.PI * 2);
      ctx.fillStyle = T('--c-chart-line'); ctx.fill();
      ctx.fillStyle = T('--c-chart-line'); ctx.font = `600 9px Inter`;
      ctx.fillText(d.line.toFixed(1) + '%', x, Yr(d.line) - 10);
    });
  }

  /* ---------- 期权 P&L（到期损益曲线 + 盈亏零线） ---------- */
  function drawPnL(canvas, spot, strike, premium, type = 'call') {
    const { ctx, w, h } = setup(canvas);
    ctx.clearRect(0, 0, w, h);
    const padL = 8, padR = 8, padT = 10, padB = 18;
    const xs = [], ys = [];
    const lo = spot * 0.85, hi = spot * 1.15;
    for (let i = 0; i <= 100; i++) {
      const p = lo + (hi - lo) * i / 100;
      const v = type === 'call' ? Math.max(p - strike, 0) - premium : Math.max(strike - p, 0) - premium;
      xs.push(p); ys.push(v * 100);
    }
    const vLo = Math.min(...ys), vHi = Math.max(...ys);
    const X = i => padL + (i / 100) * (w - padL - padR);
    const Y = v => padT + (1 - (v - vLo) / (vHi - vLo)) * (h - padT - padB);
    // 零线
    ctx.strokeStyle = 'rgba(255,255,255,.25)'; ctx.setLineDash([3, 3]);
    ctx.beginPath(); ctx.moveTo(padL, Y(0)); ctx.lineTo(w - padR, Y(0)); ctx.stroke();
    ctx.setLineDash([]);
    // 填充
    ctx.beginPath();
    ctx.moveTo(X(0), Y(ys[0]));
    ys.forEach((v, i) => ctx.lineTo(X(i), Y(v)));
    ctx.lineTo(X(100), Y(vLo)); ctx.lineTo(X(0), Y(vLo)); ctx.closePath();
    const grad = ctx.createLinearGradient(0, padT, 0, h - padB);
    grad.addColorStop(0, T('--up-btn') + '40');
    grad.addColorStop(0.5, 'transparent');
    grad.addColorStop(1, T('--down-btn') + '40');
    ctx.fillStyle = grad; ctx.fill();
    // 曲线
    ctx.beginPath();
    ys.forEach((v, i) => i ? ctx.lineTo(X(i), Y(v)) : ctx.moveTo(X(i), Y(v)));
    ctx.strokeStyle = T('--up-btn'); ctx.lineWidth = 1.6; ctx.stroke();
    // 现价标记
    const si = (spot - lo) / (hi - lo) * 100;
    ctx.setLineDash([3, 3]);
    ctx.strokeStyle = T('--price-line');
    ctx.beginPath(); ctx.moveTo(X(si), padT); ctx.lineTo(X(si), h - padB); ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = T('--text-tertiary'); ctx.font = `9px ${T('--font-mono') || 'monospace'}`;
    ctx.fillText(fmt(lo, 0), padL, h - 5);
    ctx.textAlign = 'right'; ctx.fillText(fmt(hi, 0), w - padR, h - 5);
    ctx.textAlign = 'center';
    ctx.fillText('现价 ' + fmt(spot), X(si), padT - 2);
  }

  /* ---------- 回测净值曲线 ---------- */
  function drawEquity(canvas, data) {
    drawSpark(canvas, data, { color: T('--c-accent') });
  }

  return { drawTreemap, drawSpark, drawMiniK, CandleChart, drawSankey, drawCombo, drawPnL, drawEquity, LEVELS, fmt, pct, setup, T };
})();
