/* HOTCRAFTFLOW // NIGHT SHIFT —— 引擎
   纪律：全部动态只动 transform/opacity/数值（总论#6/#7）；
   数字一律 lerp + tabular-nums（GrokBot 决策5 / 总论#4）；
   节奏三档错相：秒级走针 / 呼吸级 / 泊松事件级（SPIDERORB §微动态）。 */

'use strict';
const $ = s => document.querySelector(s);

/* ---------- 通用：lerp 计数器（GrokBot 决策5：平滑涨 = 系统活着） ---------- */
const lerps = new Map();
function lerpTo(id, target, fmt){
  const el = typeof id === 'string' ? $(id) : id;
  lerps.set(el, { target, fmt: fmt || (v => Math.round(v).toLocaleString('en-US')), cur: lerps.get(el)?.cur ?? 0 });
}
function tickLerps(){
  lerps.forEach((o, el) => {
    o.cur += (o.target - o.cur) * 0.12;            // 文档实测系数 0.12
    if (Math.abs(o.target - o.cur) < 0.5) o.cur = o.target;
    el.textContent = o.fmt(o.cur);
  });
  requestAnimationFrame(tickLerps);
}
requestAnimationFrame(tickLerps);
const fmtMoney = v => '$' + v.toFixed(2);

/* ---------- 状态 ---------- */
const S = { sources: 12840, drafts: 286, pending: 2, posts: 34, rev: 64.12, cost: 11.40, budget: 46 };
function pushStats(){
  lerpTo('#stSources', S.sources); lerpTo('#stDrafts', S.drafts); lerpTo('#stPending', S.pending);
  lerpTo('#ledPosts', S.posts);
  lerpTo('#kRev', S.rev, fmtMoney); lerpTo('#kCost', S.cost, fmtMoney); lerpTo('#kProfit', S.rev - S.cost, fmtMoney);
}
pushStats();

/* ---------- UTC 走秒（SPIDERORB §6.1#2 真实走秒） ---------- */
setInterval(() => {
  const d = new Date();
  $('#clock').textContent =
    String(d.getUTCHours()).padStart(2,'0') + ':' + String(d.getUTCMinutes()).padStart(2,'0') +
    ':' + String(d.getUTCSeconds()).padStart(2,'0') + ' UTC';
}, 1000);
$('#nextSlot').textContent = '00:40';

/* ---------- 左栏 PLATFORMS（发丝线行 + 色条 + 暗态预置行 GrokBot 决策6） ---------- */
const PLATFORMS = [
  { n:'X / EN',      s:'posting · window open', c:'var(--cyan)',   v:12, t:'ok',   ts:'LIVE' },
  { n:'Bilibili',    s:'video queue · 2 staged',  c:'var(--pink)',  v:5,  t:'ok',   ts:'LIVE' },
  { n:'Zhihu',       s:'long-form cycle',         c:'var(--blue)',  v:6,  t:'ok',   ts:'LIVE' },
  { n:'X / JP',      s:'translation loop',        c:'var(--purple)',v:4,  t:'ok',   ts:'LIVE' },
  { n:'Douyin',      s:'awaiting your OK',        c:'var(--amber)', v:2,  t:'wait', ts:'YOU'  },
  { n:'Reddit',      s:'cooldown 22 min',         c:'var(--green)', v:3,  t:'ok',   ts:'LIVE' },
  { n:'Newsletter',  s:'weekly slot · fri',       c:'var(--tx3)',   v:1,  t:'ok',   ts:'LIVE' },
  { n:'YouTube',     s:'not connected',           c:'var(--tx4)',   v:0,  t:'idle', ts:'IDLE', dim:true },
  { n:'Instagram',   s:'not connected',           c:'var(--tx4)',   v:0,  t:'idle', ts:'IDLE', dim:true },
];
$('#platforms').innerHTML = PLATFORMS.map(p => `
  <div class="rrow${p.dim ? ' dim' : ''}" style="--rc:${p.c}">
    <span class="dot"></span>
    <span><div class="nm">${p.n}</div><div class="sub">${p.s}</div></span>
    <span class="tagx ${p.t}" style="margin-left:auto">${p.ts}</span>
    <span class="rg"><div class="v num">${p.v}</div><div class="s">POSTS</div></span>
  </div>`).join('');

/* ---------- 中栏 PIPELINE：点链（GrokBot 决策3） + 含蓄hub ---------- */
const AGENTS = [
  { code:'HAR', name:'HARVEST', c:'#41D2F3', x:.10, y:.50 },
  { code:'RIS', name:'RISK',    c:'#F28383', x:.30, y:.24 },
  { code:'WRI', name:'WRITER',  c:'#A489FA', x:.50, y:.50 },
  { code:'SCH', name:'SCHED',   c:'#4C86F9', x:.70, y:.24 },
  { code:'PUB', name:'PUBLISH', c:'#52D89D', x:.88, y:.50 },
];
const LINKS = [[0,1],[1,2],[2,3],[3,4],[0,2],[2,4]];  // 主干 + 两条支线
const pipe = $('#pipe'), pctx = pipe.getContext('2d');
let pw = 0, ph = 0, dpr = Math.min(devicePixelRatio || 1, 2);
function ensurePipe(){
  const w = pipe.clientWidth, h = pipe.clientHeight;
  if (w && h && (w !== pw || h !== ph)){
    pw = w; ph = h;
    pipe.width = pw * dpr; pipe.height = ph * dpr;
    pctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
}
const flow = LINKS.map(() => Math.random());
function drawPipe(t){
  ensurePipe();
  if (!pw) return requestAnimationFrame(drawPipe);
  pctx.clearRect(0, 0, pw, ph);
  const P = AGENTS.map(a => ({ ...a, x: a.x * pw, y: a.y * ph }));
  // 结构底轨：发丝线（静止的结构用线）
  pctx.strokeStyle = 'rgba(255,255,255,.05)'; pctx.lineWidth = 1;
  LINKS.forEach(([a, b]) => {
    const A = P[a], B = P[b], mx = (A.x + B.x) / 2;
    pctx.beginPath(); pctx.moveTo(A.x, A.y);
    pctx.bezierCurveTo(mx, A.y, mx, B.y, B.x, B.y); pctx.stroke();
  });
  // 活的流量用点：r=2、gap=6、下游实体色（GrokBot 实测参数）
  LINKS.forEach(([a, b], i) => {
    const A = P[a], B = P[b], col = AGENTS[b].c;
    const N = 9, off = (t / 1400 + flow[i]) % 1;
    pctx.fillStyle = col;
    for (let k = 0; k < N; k++){
      const u = (k / N + off) % 1, mx = (A.x + B.x) / 2;
      const x = (1-u)**3 * A.x + 3*(1-u)**2*u * mx + 3*(1-u)*u*u * mx + u**3 * B.x;
      const y = (1-u)**3 * A.y + 3*(1-u)**2*u * A.y + 3*(1-u)*u*u * B.y + u**3 * B.y;
      pctx.globalAlpha = .25 + .75 * Math.sin(u * Math.PI);   // 中段最亮，两端衰减
      pctx.beginPath(); pctx.arc(x, y, 2, 0, 7); pctx.fill();
    }
    pctx.globalAlpha = 1;
  });
  // 节点：代号 + 呼吸（breathe 2s 档） + 唯一 hub 辉光特许给 PUB
  P.forEach((a, i) => {
    const br = .55 + .45 * Math.sin(t / 1000 + i * 1.7);
    if (a.code === 'PUB'){
      pctx.shadowColor = a.c; pctx.shadowBlur = 14 * br;
    }
    pctx.fillStyle = a.c;
    pctx.fillRect(a.x - 14, a.y - 14, 28, 28);                // ■ 方砖（SPIDERORB 决策9 兵种）
    pctx.shadowBlur = 0;
    pctx.fillStyle = 'rgba(12,12,24,.85)';
    pctx.fillRect(a.x - 14, a.y - 14, 28, 28);
    pctx.strokeStyle = a.c; pctx.lineWidth = 1.2;
    pctx.strokeRect(a.x - 14, a.y - 14, 28, 28);
    pctx.fillStyle = a.c; pctx.font = '700 9px JetBrains Mono, Consolas, monospace';
    pctx.textAlign = 'center'; pctx.fillText(a.code, a.x, a.y + 3);
    pctx.fillStyle = 'rgba(233,231,239,.65)'; pctx.font = '500 8.5px JetBrains Mono, Consolas, monospace';
    pctx.fillText(a.name, a.x, a.y + 30);
  });
  requestAnimationFrame(drawPipe);
}
requestAnimationFrame(drawPipe);

/* 吞吐字幕 lerp */
setInterval(() => {
  ['#tp0','#tp1','#tp2','#tp3'].forEach((id, i) => lerpTo(id, 6 + Math.random() * 14 + (i===0?20:0)));
}, 2600);

/* ---------- MONEY 双折线（GrokBot 决策8：线间距即指标） ---------- */
const mc = $('#money'), mctx = mc.getContext('2d');
const SER = { g: [], p: [] };
for (let i = 0; i < 48; i++){ SER.g.push(30 + i * 1.2 + Math.random() * 8); SER.p.push(9 + i * .3 + Math.random() * 5); }
setInterval(() => { SER.g.push(SER.g.at(-1) + .6 + Math.random() * 2.2); SER.g.shift();
                    SER.p.push(Math.max(6, SER.p.at(-1) + (Math.random() - .42) * 1.4)); SER.p.shift(); }, 1600);
function drawMoney(){
  const r = mc.getBoundingClientRect();
  if (!r.width) return requestAnimationFrame(drawMoney);
  if (mc.width !== r.width * dpr){ mc.width = r.width * dpr; mc.height = r.height * dpr; }
  mctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  const w = r.width, h = r.height;
  const line = (arr, col, fill) => {
    mctx.beginPath();
    arr.forEach((v, i) => { const x = i / (arr.length - 1) * w, y = h - v * h / 90; i ? mctx.lineTo(x, y) : mctx.moveTo(x, y); });
    mctx.strokeStyle = col; mctx.lineWidth = 1.5; mctx.stroke();
    if (fill){
      mctx.lineTo(w, h); mctx.lineTo(0, h); mctx.closePath();
      mctx.fillStyle = 'rgba(82,216,157,.07)'; mctx.fill();   // 只填绿线下方
    }
  };
  line(SER.p, '#F25E97'); line(SER.g, '#52D89D', true);
  requestAnimationFrame(drawMoney);
}
requestAnimationFrame(drawMoney);

/* ---------- 决策队列 ---------- */
const VERB = { pub:['PUB','v-pub'], blk:['BLOCK','v-blk'], skp:['SKIP','v-skp'] };
let qid = 0;
function qrow(verb, title, meta, spin, pending){
  const d = document.createElement('div');
  d.className = 'qrow' + (pending ? ' pending' : '');
  d.dataset.qid = ++qid;
  d.innerHTML = `<span class="verb ${VERB[verb][1]}">${pending ? 'SIGN' : VERB[verb][0]}</span>
    <span style="min-width:0"><div class="tt">${title}</div><div class="mt">${meta}</div></span>
    <span class="spin num">${spin}</span>
    ${pending ? '<button class="btn" style="flex:none;margin-left:8px" onclick="approve(event,' + qid + ')">APPROVE</button>' : ''}`;
  return d;
}
const Q = $('#queue');
[ ['blk','repost farm · 12 accounts','[RIS] dup-hash 0.97 · auto','·SPIN 0.04'],
  ['pub','ASM-2281 zhihu long-form','[WRI] tone 0.92 · passes','·SPIN 0.88'],
  ['skp','trending #ai-video','[SCH] slot crowded · defer','·SPIN 0.31'],
  ['pub','ASM-2276 x/en short','[PUB] window 22:00 hit','·SPIN 0.76'],
  ['blk','scraped listicle · medium','[RIS] source off-list','·SPIN 0.11'],
  ['pub','ASM-2272 bilibili cut','[SCH] staged 21:30','·SPIN 0.71'],
].forEach(r => Q.appendChild(qrow(r[0], r[1], r[2], r[3])));

/* 审批四件套（GrokBot §动态：demo 独奏 vs 产品合唱的分水岭） */
function spawnPending(){
  const topics = [
    ['douyin cut ASM-' + (2280 + qid), '[RIS] 0 flags · tone 0.90 · pays upfront'],
    ['x/jp thread ASM-' + (2280 + qid), '[WRI] draft ready · window 23:00'],
    ['zhihu answer ASM-' + (2280 + qid), '[SCH] gap slot 00:40 · low risk'],
  ];
  const t = topics[qid % topics.length];
  Q.prepend(qrow('pub', t[0], t[1], '·SPIN ' + (0.6 + Math.random() * .3).toFixed(2), true));
  S.pending++; pushStats();
  log('RIS', 'draft <b>' + t[0].split(' ')[0] + '</b> passes · waiting for <b>you</b>');
  syncGate();
}
function approve(ev, id){
  ev.stopPropagation();
  const row = Q.querySelector(`[data-qid="${id}"]`);
  if (!row) return;
  row.classList.remove('pending'); row.classList.add('done-pub','flash');
  row.querySelector('.verb').className = 'verb v-pub'; row.querySelector('.verb').textContent = 'PUB';
  const b = row.querySelector('button'); if (b) b.remove();
  S.pending = Math.max(0, S.pending - 1);
  S.posts++; S.rev += 1.2 + Math.random() * 2.6; S.cost += .3 + Math.random();
  pushStats();
  log('YOU', 'approved <b>' + row.querySelector('.tt').textContent + '</b> · traffic on');
  log('OK', 'checkpoint · ' + S.posts + ' posts · <span class="amt">' + fmtMoney(S.rev - S.cost) + '</span> so far');
  syncGate();
}
window.approve = approve;
function syncGate(){
  const g = $('#gate');
  if (S.pending > 0){
    g.classList.remove('idle'); g.classList.add('arm');
    $('#gateText').textContent = S.pending + ' draft' + (S.pending > 1 ? 's' : '') + ' need' + (S.pending > 1 ? '' : 's') + ' your sign-off';
  } else {
    g.classList.add('idle'); g.classList.remove('arm');
    $('#gateText').textContent = 'queue clear · guard watching';
  }
}
$('#gateApprove').addEventListener('click', () => {
  const top = Q.querySelector('.qrow.pending');
  if (top) approve(new Event('x'), +top.dataset.qid);
});
syncGate();

/* ---------- 日志流（SPIDERORB §6.2②：600-1200ms 随机推入） ---------- */
const LOGEL = $('#log');
function log(code, html){
  const d = document.createElement('div');
  const n = new Date();
  d.className = 'logline enter';
  d.innerHTML = `<span class="t num">${String(n.getUTCHours()).padStart(2,'0')}:${String(n.getUTCMinutes()).padStart(2,'0')}:${String(n.getUTCSeconds()).padStart(2,'0')}</span>` +
    (code === 'OK' ? `<span class="code c-OK">✓</span>` : `<span class="code c-${code}">[${code}]</span>`) + html;
  LOGEL.prepend(d);
  while (LOGEL.children.length > 26) LOGEL.lastChild.remove();
}
const CHATTER = [
  ['HAR','scanned <b>+G</b> sources · dup-hash clean'], ['WRI','note drafted · <b>G words</b>'],
  ['RIS','PASS · 0 flags · tone <b>0.G</b>'], ['SCH','indexed slot <b>G:G0</b> · value +G%'],
  ['PUB','window hit · <b>G</b> queued across matrix'], ['HAR','followed account · P(edge) 0.G'],
];
function chatter(){
  const t = CHATTER[Math.floor(Math.random() * CHATTER.length)];
  log(t[0], t[1].replace(/G/g, () => (1 + Math.floor(Math.random() * 9))).replace(/:(\d)0\b/, ':$1' + Math.floor(Math.random()*10)));
  S.sources += 1 + Math.floor(Math.random() * 6);
  if (Math.random() < .4) S.drafts += 1;
  pushStats();
  setTimeout(chatter, 900 + Math.random() * 1100);       // 泊松抖动，杜绝机器心跳
}
log('OK','night shift resumed · guardline 2 slots · <span class="amt">' + fmtMoney(S.rev - S.cost) + '</span>');
log('RIS','blocked repost · dup-hash 0.97 · 12 accounts');
log('HAR','scanned 128 sources · x/bilibili/zhihu');
chatter();
/* 审批事件：7-11s 泊松 */
(function loop(){ setTimeout(() => { spawnPending(); loop(); }, 7000 + Math.random() * 4000); })();

/* ---------- 质感开关（SPIDERORB 决策5 A/B 对照装置） ---------- */
$('#fxToggle').addEventListener('click', () => {
  document.body.classList.toggle('nofx');
  $('#fxToggle').textContent = document.body.classList.contains('nofx') ? 'TEXTURE OFF' : 'TEXTURE ON';
});

/* ---------- DOC CHECK：论断 → 落点自证 ---------- */
const CHECKS = [
  ['近黑不是纯黑，底色带青灰倾向', '总论#1 · body/卡片底', 'PASS', '.win'],
  ['发丝线分层，不用卡片间距', '总论#2 · 左栏/队列/日志行', 'PASS', '[data-anchor="2"]'],
  ['一色一语义，零例外（双轨：语义/交互）', '总论#3 · 长桥决策1', 'PASS', '.topbar'],
  ['数字 lerp + tabular-nums，值跳版不跳', 'GrokBot 决策5 · 总论#4', 'PASS', '#clock'],
  ['活的流量用点链，静的结构用发丝线', 'GrokBot 决策3', 'PASS', '#pipe'],
  ['情报带静态，不跑马灯', 'SPIDERORB 决策8', 'PASS', '.tape'],
  ['85:15 霓虹配比', 'SPIDERORB 决策1', 'PASS', '.win'],
  ['待审批 dotted 琥珀框＝未定稿', 'GrokBot 决策4', 'PASS', '#queue'],
  ['AWAIT USER 金门＝全场唯一实体钮', 'DOTS 决策6', 'PASS', '#gate'],
  ['质感四层 ≤5%（开关对照）', 'SPIDERORB 决策5', 'PASS', '#fxToggle'],
  ['暗态预置行：集合已知，成员待激活', 'GrokBot 决策6', 'PASS', '[data-anchor="2"]'],
  ['审批事件四件套合唱（金门/队列/日志/数字）', 'GrokBot §动态', 'PASS', '#gate'],
  ['LED 大计数器＝唯二辉光特许', 'SPIDERORB 决策4', 'PASS', '[data-anchor="led"]'],
];
$('#checkList').innerHTML = CHECKS.map((c, i) => `
  <div class="chk" data-i="${i}">
    <span class="n">${String(i + 1).padStart(2, '0')}</span>
    <span class="tx">${c[0]}<small>${c[1]}</small></span>
    <span class="st ${c[2] === 'PASS' ? 'pass' : 'adj'}">${c[2]}</span>
  </div>`).join('');
document.querySelectorAll('.chk').forEach(el => el.addEventListener('click', e => {
  e.stopPropagation();
  document.querySelectorAll('.anchor').forEach(a => a.classList.remove('anchor'));
  const t = document.querySelector(CHECKS[+el.dataset.i][3]);
  if (t) t.classList.add('anchor');
}));
$('#checkHead').addEventListener('click', () => {
  const c = $('#check').classList.toggle('closed');
  $('#checkHint').textContent = c ? '13 项 · 点击展开' : '13 项 · 点击锚定';
});
if (location.search.includes('check=open')){
  $('#check').classList.remove('closed');
  $('#checkHint').textContent = '13 项 · 点击锚定';
}
