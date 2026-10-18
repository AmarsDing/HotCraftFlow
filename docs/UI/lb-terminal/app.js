/* ==========================================================================
   app.js — 终端逻辑：视图路由 / 交互 / 实时模拟 / AI 面板 / 命令面板
   ========================================================================== */

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];
const fmt = Charts.fmt, pct = Charts.pct;

/* ============================== Shell 骨架 ============================== */
const VIEWS = ['market', 'heatmap', 'watchlist', 'stock', 'financial', 'options', 'ai', 'quant'];
let current = 'market';
let stockChart = null;
const candlesNVDA = genCandles(7, 160, 150, 0.018);
// 调整最后价格贴近录屏值
candlesNVDA[candlesNVDA.length - 1].c = 186.25;

function icon(name) {
  const I = {
    home: '<svg viewBox="0 0 24 24"><path d="M3 11l9-8 9 8v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M9 22V12h6v10"/></svg>',
    market: '<svg viewBox="0 0 24 24"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/></svg>',
    stock: '<svg viewBox="0 0 24 24"><path d="M3 17l5-5 4 4 6-7"/><path d="M14 9h4v4"/></svg>',
    news: '<svg viewBox="0 0 24 24"><path d="M4 5h13a2 2 0 0 1 2 2v12H6a2 2 0 0 1-2-2z"/><path d="M8 9h7M8 13h7M8 17h4"/></svg>',
    fin: '<svg viewBox="0 0 24 24"><path d="M4 19V5M4 19h16"/><path d="M8 15l3-4 3 2 4-6"/></svg>',
    opt: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 3"/></svg>',
    quant: '<svg viewBox="0 0 24 24"><path d="M8 6l-5 6 5 6M16 6l5 6-5 6"/></svg>',
    search: '<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4"/></svg>',
    sidebar: '<svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/></svg>',
    hist: '<svg viewBox="0 0 24 24"><path d="M3 12a9 9 0 1 1 3 6.7"/><path d="M3 12V7m0 5h5"/></svg>',
    back: '<svg viewBox="0 0 24 24"><path d="M15 5l-7 7 7 7"/></svg>',
    fwd: '<svg viewBox="0 0 24 24"><path d="M9 5l7 7-7 7"/></svg>',
    bell: '<svg viewBox="0 0 24 24"><path d="M18 8a6 6 0 1 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9"/><path d="M10 21a2 2 0 0 0 4 0"/></svg>',
    down: '<svg viewBox="0 0 24 24"><path d="M6 9l6 6 6-6"/></svg>',
    plus: '<svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>',
    ai: '<svg viewBox="0 0 24 24"><path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z"/><path d="M19 16l.9 2.1L22 19l-2.1.9L19 22l-.9-2.1L16 19l2.1-.9z"/></svg>',
    send: '<svg viewBox="0 0 24 24" fill="currentColor" stroke="none"><path d="M4 12l16-7-6 16-3-7z"/></svg>',
    gear: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.2a1.6 1.6 0 0 0-1-1.4 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.2a1.6 1.6 0 0 0 1.4-1 1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3h.2a1.6 1.6 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.2a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8v.2a1.6 1.6 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.2a1.6 1.6 0 0 0-1.4 1z"/></svg>',
  };
  return I[name] || '';
}

function buildShell() {
  const app = $('#app');
  app.innerHTML = `
    <div class="topbar winrow">
      <div class="traffic"><i class="r"></i><i class="y"></i><i class="g"></i></div>
      <div class="winbtn" title="收起侧栏">${icon('sidebar')}</div>
      <div class="winbtn" title="历史">${icon('hist')}</div>
      <div class="winbtn">${icon('back')}</div>
      <div class="winbtn">${icon('fwd')}</div>
      <div class="search">${icon('search')}<span>搜索股票 / 功能 / 资讯</span><kbd>Ctrl K</kbd></div>
      <div class="winbtn">${icon('bell')}</div>
      <div class="acct"><span class="av">D</span>Demo A/C(1757)${icon('down')}</div>
      <div class="btn-ai" id="btnAi">${icon('ai')}AI 助手</div>
    </div>
    <div class="topbar brandrow">
      <div class="logo"><span class="mark"></span>Longbridge</div>
      <div class="market-tabs" id="mktTabs">
        <span class="tab on">US</span><span class="tab">HK</span><span class="tab">CN</span><span class="tab">SG</span>
      </div>
    </div>
    <div class="topbar tabrow" id="viewTabs">
      <span class="tab on" data-v="market">市场总览</span>
      <span class="tab" data-v="heatmap">热力图</span>
      <span class="tab" data-v="watchlist">自选</span>
      <span class="tab" data-v="stock">个股</span>
      <span class="tab" data-v="financial">财务</span>
      <span class="tab" data-v="options">期权</span>
      <span class="tab" data-v="quant">量化</span>
    </div>
    <div class="rail" id="rail">
      <div class="ri on">${icon('home')}<span class="tip">工作台</span></div>
      <div class="ri">${icon('market')}<span class="tip">市场</span></div>
      <div class="ri">${icon('stock')}<span class="tip">行情</span><span class="badge">3</span></div>
      <div class="ri">${icon('news')}<span class="tip">资讯</span></div>
      <div class="ri">${icon('fin')}<span class="tip">财务</span></div>
      <div class="ri">${icon('opt')}<span class="tip">期权</span></div>
      <div class="ri">${icon('quant')}<span class="tip">量化</span></div>
      <div class="spacer"></div>
      <div class="ri" id="btnCmd">${icon('gear')}<span class="tip">设置 · Ctrl K</span></div>
      <div class="me">D</div>
    </div>
    <div class="viewport" id="viewport"></div>
    <div class="statusbar" id="statusbar"></div>
    <div id="toasts"></div>
    <div class="ai-drawer" id="aiDrawer"></div>
  `;

  // 状态栏
  const sb = $('#statusbar');
  function renderStatus() {
    sb.innerHTML = DB.indices.map(i => `
      <span class="idx"><b>${i.name}</b>
      <span class="num ${i.pct >= 0 ? 'up' : 'down'}">${fmt(i.val)}</span>
      <span class="num ${i.pct >= 0 ? 'up' : 'down'}">${pct(i.pct)}</span></span>
    `).join('') + `
      <span class="dot-conn" title="行情连接正常"></span>
      <span>已连接 · 延迟 38ms</span>
      <span class="clock num" id="clock"></span>`;
  }
  renderStatus();
  setInterval(() => {
    const el = $('#clock');
    if (el) el.textContent = 'US 服务器时间 ' + new Date().toLocaleTimeString('en-GB');
  }, 1000);

  $('#mktTabs').addEventListener('click', e => {
    const t = e.target.closest('.tab'); if (!t) return;
    $$('#mktTabs .tab').forEach(x => x.classList.remove('on'));
    t.classList.add('on');
  });
  $('#viewTabs').addEventListener('click', e => {
    const t = e.target.closest('.tab'); if (!t) return;
    goto(t.dataset.v);
  });
  $('#btnCmd').addEventListener('click', openCmdk);
  $('#btnAi').addEventListener('click', toggleAI);
  document.addEventListener('keydown', e => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); openCmdk(); }
    if (e.key === 'Escape') { closeCmdk(); closeAI(); }
  });
}

/* ============================== 视图路由 ============================== */
function goto(v, opts = {}) {
  if (v === current && !opts.force) return;
  const vp = $('#viewport');
  const old = $('.view:not(.exit)', vp);
  $$('#viewTabs .tab').forEach(x => x.classList.toggle('on', x.dataset.v === v));
  current = v;
  const el = document.createElement('div');
  el.className = 'view enter';
  el.dataset.v = v;
  vp.appendChild(el);
  RENDER[v](el);
  if (old) { old.classList.add('exit'); setTimeout(() => old.remove(), 950); }
  setTimeout(() => el.classList.remove('enter'), 950);
}

const RENDER = {};

/* ============================== 视图：Market ============================== */
RENDER.market = (el) => {
  el.innerHTML = `
    <div class="market">
      <div class="panel idx-card full">
        <div class="idx-tabs"><span class="tab underline on">标普500</span><span class="tab underline">纳斯达克</span><span class="tab underline">道琼斯</span></div>
        <div class="idx-val num down">6,715.79</div>
        <div class="idx-chg"><span class="num down">-8.95</span><span class="num down">-0.13%</span><span class="dim">10/16 收盘</span></div>
        <div class="idx-spark"><canvas id="idxSpark"></canvas></div>
      </div>
      <div class="panel monitor">
        <div class="panel-hd">Monitor <span class="sub">实时异动</span><span style="margin-left:auto" class="chip on">全部</span><span class="chip">大涨</span><span class="chip">大跌</span></div>
        <div id="monList"></div>
      </div>
      <div style="display:flex;flex-direction:column;gap:10px">
        <div class="panel"><div class="panel-hd">行业涨幅榜</div>
          <div class="industries">${DB.industries.map(g => `
            <div class="ind-col"><h4>${g.name}<span class="num ${g.chg >= 0 ? 'up' : 'down'}">${pct(g.chg)}</span></h4>
            ${g.items.map(([n, c]) => `<div class="ir"><span class="mut">${n}</span><span class="num ${c >= 0 ? 'up' : 'down'}">${pct(c)}</span></div>`).join('')}
            </div>`).join('')}
          </div>
        </div>
        <div class="panel"><div class="panel-hd">Quote List <span class="sub">活跃榜</span></div>
          <table class="t zebra">
            <thead><tr><th>代码</th><th>名称</th><th class="r">最新</th><th class="r">涨跌幅</th><th class="r">成交量</th><th class="r">成交额</th></tr></thead>
            <tbody>${DB.quoteList.map(q => `
              <tr><td class="fw6">${q.sym}</td><td class="mut">${q.name}</td>
              <td class="r num fw6">${fmt(q.px)}</td>
              <td class="r num ${q.chg >= 0 ? 'up' : 'down'}">${pct(q.chg)}</td>
              <td class="r num mut">${q.vol}</td><td class="r num mut">${q.amt}</td></tr>`).join('')}
            </tbody>
          </table>
        </div>
      </div>
    </div>`;
  Charts.drawSpark($('#idxSpark', el), genSpark(11), { color: Charts.T('--down-btn') });
  const mon = $('#monList', el);
  DB.monitorSeeds.forEach((m, i) => setTimeout(() => {
    const d = document.createElement('div');
    d.className = 'mon-row';
    d.innerHTML = `<span class="t">${new Date().toLocaleTimeString('en-GB')}</span>
      <span class="badge ${m.dir}">${m.dir === 'up' ? '涨' : '跌'}</span>
      <span class="nm">${m.sym}<small>${m.name}</small></span>
      <span class="v ${m.dir}">${m.v}</span>
      <span class="dim">异动提醒</span>`;
    mon.prepend(d);
  }, 300 + i * 420));
};

/* ============================== 视图：Heatmap ============================== */
let hmThreshold = null;
RENDER.heatmap = (el) => {
  el.innerHTML = `
    <div class="heatmap">
      <div class="hm-bar">
        <span class="hm-title">热力图</span>
        <div class="chips" id="hmIdx">
          <span class="chip on">标普500</span><span class="chip">纳斯达克100</span><span class="chip">道琼斯</span><span class="chip">全部美股</span>
        </div>
        <span class="sel">市值 ${icon('down')}</span>
        <span class="sel">涨跌幅 ${icon('down')}</span>
        <span class="sel">当日 ${icon('down')}</span>
        <div class="hm-thresholds" id="hmThr">
          <span class="dim" style="font-size:10px;margin-right:4px">阈值筛选</span>
          ${Charts.LEVELS.map(l => `<span class="thr" data-l="${l}" style="background:${l >= 4 ? 'rgba(12,168,144,.9)' : l >= 2 ? 'rgba(12,132,108,.9)' : 'rgba(12,108,96,.9)'}">≥${l}%</span>`).join('')}
        </div>
      </div>
      <div class="hm-wrap"><canvas id="hmCanvas"></canvas></div>
    </div>`;
  const cv = $('#hmCanvas', el);
  let tiles = [];
  const paint = () => { tiles = Charts.drawTreemap(cv, DB.heatmap); };
  requestAnimationFrame(paint);
  new ResizeObserver(paint).observe(cv.parentElement);

  // 阈值筛选
  $('#hmThr', el).addEventListener('click', e => {
    const t = e.target.closest('.thr'); if (!t) return;
    const l = +t.dataset.l;
    hmThreshold = hmThreshold === l ? null : l;
    $$('#hmThr .thr', el).forEach(x => x.classList.toggle('off', hmThreshold !== null && +x.dataset.l !== hmThreshold));
    paint();
    if (hmThreshold) {
      // 低亮非命中
      const { ctx } = Charts.setup(cv);
      tiles.forEach(n => {
        if (Math.abs(n.chg) < hmThreshold) {
          ctx.fillStyle = 'rgba(24,25,29,.72)';
          ctx.fillRect(n.x, n.y, n.w, n.h);
        }
      });
    }
  });

  // hover 浮卡
  const card = document.createElement('div');
  card.className = 'hm-card'; card.style.display = 'none';
  el.appendChild(card);
  cv.addEventListener('mousemove', e => {
    const r = cv.getBoundingClientRect();
    const x = e.clientX - r.left, y = e.clientY - r.top;
    const t = tiles.find(n => x >= n.x && x <= n.x + n.w && y >= n.y && y <= n.y + n.h);
    if (!t) { card.style.display = 'none'; return; }
    const w = DB.watchlist.find(v => v.sym === t.name);
    card.style.display = 'block';
    card.innerHTML = `
      <div class="hd"><span class="lg" style="background:${t.logoBg}">${t.name.slice(0, 2)}</span>
      <div><div class="tk">${t.name}</div><div class="co">${w ? w.name : ''} · ${t.group}</div></div></div>
      <div class="kv"><span>最新价</span><b>$${w ? fmt(w.px) : '—'}</b></div>
      <div class="kv"><span>涨跌幅</span><b class="${t.chg >= 0 ? 'up' : 'down'}">${pct(t.chg)}</b></div>
      <div class="kv"><span>市值</span><b>$${fmt(t.size / 10, 1)}B</b></div>`;
    const flip = y > r.height * 0.6;
    card.classList.toggle('flip', flip);
    card.style.setProperty('--arrow-x', '20px');
    card.style.left = Math.min(x + 12, r.width - 205) + 'px';
    card.style.top = (flip ? y - card.offsetHeight - 12 : y + 14) + 'px';
  });
  cv.addEventListener('mouseleave', () => card.style.display = 'none');
};

/* ============================== 视图：Watchlist 宫格 ============================== */
RENDER.watchlist = (el) => {
  el.innerHTML = `
    <div class="wl">
      <div class="wl-bar">
        <span class="hm-title">自选</span>
        <span class="sel">默认分组 ${icon('down')}</span>
        <span class="seg"><span class="on">K线</span><span>分时</span><span>列表</span></span>
      </div>
      <div class="wl-grid">
        ${DB.watchlist.map((w2, i) => `
          <div class="panel wl-card" data-sym="${w2.sym}">
            <div class="ch"><span class="nm">${w2.sym}</span><span class="px num">$${fmt(w2.px)}</span>
            <span class="pc num ${w2.chg >= 0 ? 'up' : 'down'}">${pct(w2.chg)}</span></div>
            <canvas data-i="${i}"></canvas>
            <div class="cf"><span>${w2.name}</span><span style="margin-left:auto">${w2.market} · 日线</span></div>
          </div>`).join('')}
      </div>
    </div>`;
  $$('canvas', el).forEach((c, i) => Charts.drawMiniK(c, genCandles(20 + i * 7, 40, 80 + i * 13, 0.02)));
  el.addEventListener('click', e => {
    const card = e.target.closest('.wl-card');
    if (card) goto('stock', { force: true });
  });
};

/* ============================== 视图：Stock 详情 ============================== */
RENDER.stock = (el) => {
  const nvda = DB.watchlist[0];
  el.innerHTML = `
    <div class="stock">
      <div class="watch">
        <div class="wh"><span></span><span>名称</span><span>最新</span><span>涨跌幅</span></div>
        ${DB.watchlist.map(w2 => `
          <div class="wr ${w2.sym === 'NVDA' ? 'on' : ''}">
            <span class="flag">🇺🇸</span>
            <span class="nm">${w2.sym}<small>${w2.name}</small></span>
            <span class="px num ${w2.chg >= 0 ? 'up' : 'down'}">${fmt(w2.px)}</span>
            <span class="pc num ${w2.chg >= 0 ? 'up' : 'down'}">${pct(w2.chg)}</span>
          </div>`).join('')}
      </div>
      <div class="chart">
        <div class="chart-hd">
          <span class="sym">NVDA</span><span class="dim">英伟达</span>
          <span class="px num down">$186.25</span><span class="pc num down">-0.41 -0.22%</span>
          <div class="chips" style="margin-left:8px">
            <span class="chip">分时</span><span class="chip on">日K</span><span class="chip">周K</span><span class="chip">月K</span>
          </div>
          <span class="sel" style="margin-left:auto">MA EMA BOLL ${icon('down')}</span>
        </div>
        <div class="ohlc-bar" id="ohlc"><span class="dim">移动十字光标查看 OHLC</span></div>
        <div class="chart-canvas"><canvas id="mainChart"></canvas></div>
        <div class="chart-ft">
          <span class="chip on">成交量</span><span class="chip">MACD</span><span class="chip">KDJ</span><span class="chip">RSI</span>
          <span class="chip">事件</span>
          <span class="dim" style="margin-left:auto;font-size:10px">滚轮缩放 · 十字光标读数</span>
        </div>
      </div>
      <div class="quote">
        <div class="qh">
          <div class="qn">NVDA · 英伟达 · 纳斯达克</div>
          <div class="qpx num down">186.25</div>
          <div class="qchg"><span class="num down">-0.41</span><span class="num down">-0.22%</span></div>
          <div class="qtime">美东 10/16 16:00 收盘 · USD</div>
        </div>
        <div class="qacts"><i title="提醒">⏰</i><i title="加自选">☆</i><i title="分享">↗</i><i title="更多">⋯</i></div>
        <div class="kv-grid">
          ${[['今开', '185.90'], ['昨收', '186.66'], ['最高', '188.02'], ['最低', '184.12'],
             ['成交量', '1.82亿'], ['成交额', '$339.0亿'], ['市值', '$4.54万亿'], ['市盈率TTM', '52.4'],
             ['52周高', '195.62'], ['52周低', '86.62'], ['股息率', '0.02%'], ['换手率', '0.74%']]
            .map(([k, v]) => `<div class="k">${k}</div><div class="v num">${v}</div>`).join('')}
        </div>
        <div class="pressure">
          <div class="lb"><span class="up fw6">买盘 58%</span><span class="down fw6">42% 卖盘</span></div>
          <div class="pbar"><div class="pu" style="width:58%"></div><div class="pd" style="width:42%"></div>
          <div class="mid">58 : 42</div></div>
        </div>
        <div class="ts">
          <div class="th"><span>逐笔成交</span><span class="dim">Time & Sales</span></div>
          <div class="trow dim"><span>时间</span><span>价格</span><span>数量</span><span>方向</span></div>
          ${DB.timeSales.map(([t, p, v, d]) => `
            <div class="trow"><span class="dim">${t}</span>
            <span class="num ${d === 'up' ? 'up' : d === 'down' ? 'down' : 'flat'}">${fmt(p)}</span>
            <span class="num">${v.toLocaleString()}</span>
            <span class="${d === 'up' ? 'up' : d === 'down' ? 'down' : 'flat'}">${d === 'up' ? '买入' : d === 'down' ? '卖出' : '中性'}</span></div>`).join('')}
        </div>
      </div>
      <div class="trade">
        <div class="ttabs">
          <span class="tab on">买入</span><span class="tab">卖出</span><span class="tab">持仓</span><span class="tab">订单</span><span class="tab">条件单</span>
        </div>
        <div class="tform">
          <div class="tfield"><label>订单类型 <i>?</i></label>
            <span class="sel" style="width:100%">限价单 ${icon('down')}</span></div>
          <div class="tfield"><label>价格</label>
            <div class="inp"><input value="186.25"><span class="step">−</span><span class="step">+</span></div></div>
          <div class="tfield"><label>数量</label>
            <div class="inp"><input value="100"><span class="step">−</span><span class="step">+</span></div>
            <span class="hint">购买力 $48,240.50</span></div>
          <div class="tfield"><label>有效期 <i>?</i></label>
            <span class="sel" style="width:100%">当日有效 ${icon('down')}</span></div>
          <div class="trow2">
            <span class="dim" style="font-size:11px">预估金额 <b class="num" style="color:var(--text-primary)">$18,625.00</b></span>
            <span class="dim" style="font-size:11px">预估佣金 <b class="num" style="color:var(--text-primary)">$0.99</b></span>
            <span style="margin-left:auto;display:flex;gap:8px">
              <button class="btn buy" id="btnBuy">买入 NVDA</button>
              <button class="btn sell" id="btnSell">卖出 NVDA</button>
            </span>
          </div>
        </div>
      </div>
    </div>`;

  const ohlc = $('#ohlc', el);
  stockChart = new Charts.CandleChart($('#mainChart', el), candlesNVDA, {
    events: [{ candle: candlesNVDA[60], type: 'E' }, { candle: candlesNVDA[110], type: 'D' }],
    onCursor: c => {
      if (!c) { ohlc.innerHTML = '<span class="dim">移动十字光标查看 OHLC</span>'; return; }
      const chg = ((c.c - c.o) / c.o * 100);
      ohlc.innerHTML = `<b>${c.t}</b>
        <span>开 <b class="num">${fmt(c.o)}</b></span><span>高 <b class="num">${fmt(c.h)}</b></span>
        <span>低 <b class="num">${fmt(c.l)}</b></span><span>收 <b class="num ${chg >= 0 ? 'up' : 'down'}">${fmt(c.c)}</b></span>
        <span class="${chg >= 0 ? 'up' : 'down'}">${pct(chg)}</span>
        <span class="dim">量 ${c.v}</span>`;
    }
  });

  $('#btnBuy', el).addEventListener('click', () => toast('buy', 'NVDA'));
  $('#btnSell', el).addEventListener('click', () => toast('sell', 'NVDA'));
};

function toast(dir, sym) {
  const t = document.createElement('div');
  t.className = 'toast';
  t.innerHTML = `
    <div style="display:flex;align-items:center;gap:8px">
      <span class="badge ${dir === 'buy' ? 'up' : 'down'}">Filled</span>
      <b style="font-size:12px">${sym} ${dir === 'buy' ? '买入' : '卖出'}委托已成交</b>
    </div>
    <div class="dim" style="font-size:11px;margin-top:4px">100 股 @ $186.25 · 成交金额 $18,625.00 · 佣金 $0.99</div>`;
  $('#toasts').appendChild(t);
  setTimeout(() => { t.classList.add('out'); setTimeout(() => t.remove(), 220); }, 3200);
}

/* ============================== 视图：Financial ============================== */
RENDER.financial = (el) => {
  el.innerHTML = `
    <div class="fin">
      <div class="fin-tabs">
        <span class="tab underline on">财报速览</span><span class="tab underline">利润表</span>
        <span class="tab underline">资产负债表</span><span class="tab underline">现金流量表</span>
        <span class="tab underline">核心指标</span><span class="tab underline">收入构成</span>
      </div>
      <div class="card">
        <div class="card-hd"><h3>${DB.fin.headline}</h3><span class="meta">AI 财报分析 · 仅供参考</span>
        <span class="r"><span class="badge up">超预期</span><button class="btn sm">查看原文</button></span></div>
        <div class="card-bd ai-sum">${DB.fin.summary}</div>
      </div>
      <div style="display:grid;grid-template-columns:1.2fr 1fr;gap:12px">
        <div class="card">
          <div class="card-hd"><h3>利润瀑布</h3><span class="meta">FY2026 Q3 · 单位 $B</span></div>
          <div class="card-bd">
            <div class="sk-legend">
              <span><i style="background:#5060F0"></i>收入</span>
              <span><i style="background:#38C850"></i>利润</span>
              <span><i style="background:#F09818"></i>费用</span>
            </div>
            <div class="sankey-wrap"><svg id="sankey"></svg></div>
          </div>
        </div>
        <div class="card">
          <div class="card-hd"><h3>营收 vs 毛利率</h3><span class="meta">近 7 个季度</span></div>
          <div class="card-bd"><div class="combo"><canvas id="combo"></canvas></div></div>
        </div>
      </div>
      <div class="card">
        <div class="card-hd"><h3>核心财务指标</h3><span class="meta">同比口径</span></div>
        <table class="t zebra">
          <thead><tr><th>指标</th><th class="r">本季</th><th class="r">同比</th></tr></thead>
          <tbody>${DB.fin.ratios.map(([k, v, c, d]) => `
            <tr><td class="mut">${k}</td><td class="r num fw6">${v}</td>
            <td class="r num ${d}">${c}</td></tr>`).join('')}
          </tbody>
        </table>
      </div>
    </div>`;
  Charts.drawSankey($('#sankey', el), DB.fin.sankey);
  Charts.drawCombo($('#combo', el), DB.fin.combo);
};

/* ============================== 视图：Options ============================== */
RENDER.options = (el) => {
  const O = DB.options;
  el.innerHTML = `
    <div class="opt">
      <div class="opt-hd">
        <h3>${O.under} 期权链</h3>
        <span class="num down fw6">$${fmt(O.spot)}</span>
        <span class="chip on">标准</span><span class="chip">按周</span>
        <span style="margin-left:auto" class="sel">希腊值 ${icon('down')}</span>
        <span class="sel">隐含波动率 ${icon('down')}</span>
      </div>
      <div class="exp-bar">${O.exps.map((e, i) => `<span class="exp ${i === 3 ? 'on' : ''}">${e}${i === 3 ? '<sup>●</sup>' : ''}</span>`).join('')}</div>
      <div class="pressure" style="padding:0">
        <div class="lb"><span class="up fw6">Call 持仓 62%</span><span class="down fw6">38% Put</span></div>
        <div class="pbar"><div class="pu" style="width:62%"></div><div class="pd" style="width:38%"></div><div class="mid">PCR 0.61</div></div>
      </div>
      <div style="display:grid;grid-template-columns:1fr 280px;gap:10px;flex:1;min-height:0">
        <div class="chain-wrap">
          <table class="chain">
            <thead><tr>
              <th class="gh" colspan="4">CALL</th><th class="gh">行权价</th><th class="gh" colspan="4">PUT</th>
            </tr><tr>
              <th>买价</th><th>卖价</th><th>成交量</th><th>持仓量</th>
              <th></th>
              <th>买价</th><th>卖价</th><th>成交量</th><th>持仓量</th>
            </tr></thead>
            <tbody>${O.chain.map(r => `
              <tr class="${r.atm ? 'atm' : ''}">
                <td class="num up">${fmt(r.call.bid)}</td><td class="num up">${fmt(r.call.ask)}</td>
                <td class="num mut">${r.call.vol.toLocaleString()}</td><td class="num mut">${r.call.oi.toLocaleString()}</td>
                <td class="strike num">${r.strike}</td>
                <td class="num down">${fmt(r.put.bid)}</td><td class="num down">${fmt(r.put.ask)}</td>
                <td class="num mut">${r.put.vol.toLocaleString()}</td><td class="num mut">${r.put.oi.toLocaleString()}</td>
              </tr>`).join('')}
            </tbody>
          </table>
        </div>
        <div class="panel" style="display:flex;flex-direction:column">
          <div class="panel-hd">P&L 损益 <span class="sub">买入 185 Call</span></div>
          <div class="panel-bd" style="flex:1"><canvas id="pnl" style="width:100%;height:100%"></canvas></div>
        </div>
      </div>
    </div>`;
  Charts.drawPnL($('#pnl', el), O.spot, 185, 7.0, 'call');
};

/* ============================== 视图：Quant ============================== */
RENDER.quant = (el) => {
  el.innerHTML = `
    <div class="quant">
      <div class="code-wrap">
        <div class="code-tabs">
          <span class="tab on" style="border-radius:6px 6px 0 0;border:1px solid var(--border-soft);border-bottom:none">dual_momentum.py</span>
          <span class="tab">grid_trade.py</span><span class="tab">+</span>
        </div>
        <div class="editor">
          <div class="ln">${DB.quantCode.split('\n').map((_, i) => i + 1).join('<br>')}</div>
          <div class="src" id="qsrc">${highlight(DB.quantCode)}</div>
        </div>
        <div style="display:flex;gap:8px;padding-top:8px">
          <button class="btn ghost" id="btnRun">▶ 运行回测</button>
          <span class="sel">NVDA ${icon('down')}</span>
          <span class="sel">日线 ${icon('down')}</span>
          <span class="sel">2024-10 → 2026-10 ${icon('down')}</span>
        </div>
      </div>
      <div class="bt">
        <div class="panel" style="flex:1;display:flex;flex-direction:column;min-height:0">
          <div class="panel-hd">回测日志 <span class="sub" id="btState">待运行</span></div>
          <div class="bt-log" id="btLog"><div class="l dim">点击「运行回测」开始…</div></div>
        </div>
        <div class="panel equity"><div class="panel-hd">净值曲线</div>
        <div class="panel-bd" style="padding:6px"><canvas id="eqCurve"></canvas></div></div>
      </div>
    </div>`;
  Charts.drawEquity($('#eqCurve', el), genSpark(99, 60, 100));
  $('#btnRun', el).addEventListener('click', () => {
    const log = $('#btLog', el); log.innerHTML = '';
    $('#btState', el).innerHTML = '<span class="spinner"></span> 运行中';
    DB.btLog.forEach(([t, lv, msg], i) => setTimeout(() => {
      const d = document.createElement('div');
      d.className = 'l';
      d.innerHTML = `<span class="dim">${t}</span><b>[${lv}]</b><span>${msg}</span>`;
      log.appendChild(d); log.scrollTop = log.scrollHeight;
      if (i === DB.btLog.length - 1) $('#btState', el).textContent = '已完成 · 2.4s';
    }, i * 380));
  });
};

function highlight(code) {
  const esc = code.replace(/&/g, '&amp;').replace(/</g, '&lt;');
  const stash = [];
  // 占位符用纯字母（避开 \d 与标识符正则）
  const keep = (html) => { stash.push(html); return `§q${String.fromCharCode(97 + stash.length - 1)}§`; };
  // 1) 先提取注释与字符串为占位符，避免后续正则污染
  let s = esc
    .replace(/(#.*)/g, m => keep(`<span class="tok-com">${m}</span>`))
    .replace(/("[^"\n]*")/g, m => keep(`<span class="tok-str">${m}</span>`))
    .replace(/(@[\w.]+\([^)]*\))|(@[\w.]+)/g, m => keep(`<span class="tok-dec">${m}</span>`));
  // 2) 关键字 / 数字 / 函数 / 变量
  s = s
    .replace(/\b(import|class|def|if|elif|else|and|or|not|return|from|True|False|None)\b/g, '<span class="tok-kw">$1</span>')
    .replace(/\b(\d+(?:\.\d+)?)\b/g, '<span class="tok-num">$1</span>')
    .replace(/\b(sma|rsi|buy|sell|report)\b/g, '<span class="tok-fn">$1</span>')
    .replace(/\b(fast|slow|pos|weight|symbol|window|close|metrics)\b/g, '<span class="tok-var">$1</span>');
  // 3) 循环回填（装饰器内可能含字符串占位符）
  let prev;
  do { prev = s; s = s.replace(/§q([a-z])§/g, (_, c) => stash[c.charCodeAt(0) - 97]); } while (s !== prev);
  return s;
}

/* ============================== AI 助手 ============================== */
let aiOpen = false, aiFull = false;
function toggleAI() { aiOpen ? closeAI() : openAI(false); }
function openAI(full) {
  aiOpen = true; aiFull = full;
  const d = $('#aiDrawer');
  d.classList.add('open'); d.classList.toggle('full', full);
  $('#viewport').classList.add('recede');
  if (!d.dataset.built) { buildAI(d); d.dataset.built = '1'; }
}
function closeAI() {
  aiOpen = false;
  $('#aiDrawer').classList.remove('open');
  $('#viewport').classList.remove('recede');
}
function buildAI(d) {
  d.innerHTML = `
    <div class="ai-hd">
      <span class="ai-brand">Longbridge<span style="color:var(--c-accent)">AI</span></span>
      <span class="badge gray">Beta</span>
      <span class="r">
        <div class="winbtn" id="aiFull" title="全屏">${icon('market')}</div>
        <div class="winbtn" id="aiClose" title="关闭">✕</div>
      </span>
    </div>
    <div style="display:flex;flex:1;min-height:0">
      <div class="ai-hist" id="aiHist" style="display:none">
        <div class="btn ghost sm" style="width:100%;justify-content:center">+ 新会话</div>
        <div class="grp">今天</div>
        <div class="it on">英伟达财报解读</div><div class="it">高股息港股筛选</div>
        <div class="grp">本周</div>
        <div class="it">特斯拉期权分析</div><div class="it">半导体板块对比</div><div class="it">美联储议息前瞻</div>
      </div>
      <div style="display:flex;flex-direction:column;flex:1;min-width:0">
        <div class="ai-msg" id="aiMsg"></div>
        <div class="ai-inp center" id="aiInp">
          <div class="box">
            ${icon('ai')}
            <input id="aiInput" placeholder="问我任何关于市场、股票、期权的问题…" />
            <span class="send" id="aiSend">${icon('send')}</span>
          </div>
        </div>
      </div>
    </div>`;
  $('#aiClose', d).addEventListener('click', closeAI);
  $('#aiFull', d).addEventListener('click', () => {
    aiFull = !aiFull;
    d.classList.toggle('full', aiFull);
    $('#aiHist', d).style.display = aiFull ? 'block' : 'none';
  });
  $('#aiSend', d).addEventListener('click', sendAI);
  $('#aiInput', d).addEventListener('keydown', e => { if (e.key === 'Enter') sendAI(); });
  aiGreeting();
}
function aiGreeting() {
  const m = $('#aiMsg');
  m.innerHTML = `
    <div class="ai-bub bot" style="align-self:center;text-align:center;padding:18px 0 6px">
      <div class="ai-hero">你好，我是<br>Longbridge<span style="color:var(--c-accent)">AI</span></div>
      <div class="ai-grad" style="margin:10px auto 0"></div>
    </div>
    <div class="ai-sugg">
      ${DB.aiSuggests.map(s => `<div class="s" data-q="${s.q}"><b>${s.s}</b>${s.q}</div>`).join('')}
    </div>
    <div class="ai-bub bot disclaimer" style="font-size:10px;color:var(--text-tertiary)">内容由 AI 生成，仅供参考，不构成投资建议。</div>`;
  $$('#aiMsg .s').forEach(s => s.addEventListener('click', () => { $('#aiInput').value = s.dataset.q; sendAI(); }));
}
function sendAI() {
  const inp = $('#aiInput');
  const q = inp.value.trim(); if (!q) return;
  inp.value = '';
  const m = $('#aiMsg');
  m.querySelector('.ai-sugg')?.remove();
  m.insertAdjacentHTML('beforeend', `<div class="ai-bub user">${q}</div>`);
  const think = document.createElement('div');
  think.className = 'ai-bub bot thinking';
  think.innerHTML = `<span class="ai-steps"><span class="pulse"></span>正在检索行情与财报数据…</span><span class="dots"><i>●</i><i>●</i><i>●</i></span>`;
  m.appendChild(think); m.scrollTop = m.scrollHeight;
  setTimeout(() => {
    think.className = 'ai-bub bot';
    think.innerHTML = `<span id="stream"></span><span class="caret"></span>`;
    const full = '英伟达 FY2026 Q3 营收 <b>$570.1亿</b>（同比 <b class="up">+62.5%</b>），数据中心业务占比升至 89%。毛利率 73.4%，环比提升 0.7 个百分点。管理层指引下季营收 $600±10 亿，高于市场预期。';
    let i = 0;
    const tick = setInterval(() => {
      i += 2;
      $('#stream', think).innerHTML = full.slice(0, i);
      m.scrollTop = m.scrollHeight;
      if (i >= full.length) {
        clearInterval(tick);
        think.querySelector('.caret')?.remove();
        think.insertAdjacentHTML('beforeend', `
          <div class="rcard">
            <div class="rh"><span class="lg" style="background:#76B900">NV</span>
            <b>NVDA 英伟达</b><span class="px num down">$186.25</span></div>
            <canvas id="rcSpark"></canvas>
            <div style="display:flex;gap:6px">
              <span class="badge down">-0.22%</span><span class="badge gray">市值 $4.54T</span>
              <button class="btn sm ghost" style="margin-left:auto" onclick="goto('stock',{force:true});closeAI()">查看详情 →</button>
            </div>
          </div>
          <div class="disclaimer">内容由 AI 生成，仅供参考，不构成投资建议。</div>`);
        Charts.drawSpark($('#rcSpark', think), genCandles(5, 40, 150, 0.02).map(c => c.c));
        m.scrollTop = m.scrollHeight;
      }
    }, 28);
  }, 1200);
}

/* ============================== 命令面板 ============================== */
function openCmdk() {
  if ($('.cmdk')) return;
  const w = document.createElement('div');
  w.className = 'cmdk';
  w.innerHTML = `
    <div class="cmdk-box">
      <div class="cmdk-inp">${icon('search')}<input placeholder="搜索功能 / 切换主题…" id="cmdkInput"><kbd class="dim" style="font-size:9px;border:1px solid var(--border-strong);border-radius:3px;padding:0 4px">ESC</kbd></div>
      <div id="cmdkList"></div>
    </div>`;
  $('#app').appendChild(w);
  const list = $('#cmdkList', w);
  const render = (kw = '') => {
    list.innerHTML = DB.themes
      .filter(t => !kw || t.name.toLowerCase().includes(kw.toLowerCase()))
      .map(t => `<div class="cmdk-row" data-t="${t.id}"><span class="sw" style="background:${t.sw}"></span>${t.name}<span class="tag">${t.tag}</span></div>`).join('')
      || '<div class="cmdk-row dim">无匹配项</div>';
    $$('.cmdk-row[data-t]', list).forEach(r => r.addEventListener('click', () => {
      document.body.dataset.theme = r.dataset.t;
      closeCmdk();
      // 主题切换后重绘当前视图
      const v = $('.view:not(.exit)');
      if (v) { const vv = v.dataset.v; v.innerHTML = ''; RENDER[vv](v); }
    }));
  };
  render();
  $('#cmdkInput', w).addEventListener('input', e => render(e.target.value));
  w.addEventListener('click', e => { if (e.target === w) closeCmdk(); });
  $('#cmdkInput', w).focus();
}
function closeCmdk() { $('.cmdk')?.remove(); }

/* ============================== 启动 ============================== */
document.body.dataset.theme = 'default-dark';
buildShell();
goto('market', { force: true });
