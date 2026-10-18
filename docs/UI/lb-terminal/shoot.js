const { chromium } = require('C:/Users/Administrator/.workbuddy/binaries/node/workspace/node_modules/playwright');

(async () => {
  const browser = await chromium.launch({
    executablePath: 'C:/Users/Administrator/AppData/Local/ms-playwright/chromium-1234/chrome-win64/chrome.exe',
    headless: true
  });
  const errs = [];
  const p = await browser.newPage({ viewport: { width: 1320, height: 810 }, deviceScaleFactor: 1 });
  p.on('pageerror', e => errs.push('PAGEERROR: ' + e.message));
  p.on('console', m => { if (m.type() === 'error') errs.push(m.text()); });
  const url = 'file:///C:/Users/Administrator/WorkBuddy/FrameWork/docs/UI/lb-terminal/index.html';
  await p.goto(url);
  await p.waitForTimeout(1500);

  const out = 'C:/Users/Administrator/WorkBuddy/FrameWork/docs/UI/lb-terminal/shots';
  const shot = async (name) => { await p.screenshot({ path: `${out}/${name}.png` }); console.log('shot', name); };

  await shot('01_market');

  await p.evaluate(() => goto('heatmap'));
  await p.waitForTimeout(1200);
  await shot('02_heatmap');

  // hover 热力图浮卡
  await p.mouse.move(660, 500);
  await p.waitForTimeout(300);
  await shot('02b_heatmap_hover');

  await p.evaluate(() => goto('watchlist'));
  await p.waitForTimeout(1200);
  await shot('03_watchlist');

  await p.evaluate(() => goto('stock'));
  await p.waitForTimeout(1200);
  await shot('04_stock');
  // 十字光标
  await p.mouse.move(660, 420);
  await p.waitForTimeout(300);
  await shot('04b_stock_crosshair');

  await p.evaluate(() => goto('financial'));
  await p.waitForTimeout(1200);
  await shot('05_financial');

  await p.evaluate(() => goto('options'));
  await p.waitForTimeout(1200);
  await shot('06_options');

  await p.evaluate(() => goto('quant'));
  await p.waitForTimeout(800);
  await p.click('#btnRun');
  await p.waitForTimeout(3600);
  await shot('07_quant');

  // AI 抽屉
  await p.evaluate(() => goto('market', { force: true }));
  await p.waitForTimeout(800);
  await p.evaluate(() => openAI(false));
  await p.waitForTimeout(1000);
  await shot('08_ai_drawer');

  // AI 提问 + 富文本卡
  await p.evaluate(() => { document.querySelector('#aiInput').value = '英伟达最新财报的核心看点？'; sendAI(); });
  await p.waitForTimeout(6500);
  await shot('09_ai_chat');

  // AI 全屏
  await p.evaluate(() => document.querySelector('#aiFull').click());
  await p.waitForTimeout(900);
  await shot('10_ai_full');

  // 命令面板
  await p.evaluate(() => { closeAI(); openCmdk(); });
  await p.waitForTimeout(400);
  await shot('11_cmdk');

  // A股主题
  await p.evaluate(() => { document.body.dataset.theme = 'cn-red-up'; closeCmdk(); goto('heatmap', { force: true }); });
  await p.waitForTimeout(1200);
  await shot('12_cn_theme');

  console.log('ERRORS:', errs.length ? errs : 'none');
  await browser.close();
  process.exit(0);
})();
