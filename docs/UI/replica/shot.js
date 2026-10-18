// 复刻界面截图脚本：按状态逐一截图，供与原视频帧对比
const { chromium } = require("playwright");
const path = require("path");

const HTML = "file:///" + "C:/Users/Administrator/WorkBuddy/FrameWork/docs/UI/replica/index.html";
const OUT = "C:/Users/Administrator/WorkBuddy/FrameWork/docs/UI/dots-compare";

const SHOTS = [
  ["graph", "stage03", "rep_graph_stage03.png"],
  ["graph", "stage05", "rep_graph_stage05.png"],
  ["graph", "flow",    "rep_graph_flow.png"],
  ["graph", "heat",    "rep_graph_heat.png"],
  ["graph", "stage06", "rep_graph_stage06.png"],
  ["war",   "war-start", "rep_war_start.png"],
  ["war",   "war-mid",   "rep_war_mid.png"],
  ["war",   "war-final", "rep_war_final.png"],
];

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({
    viewport: { width: 1320, height: 2868 },
    deviceScaleFactor: 1,
  });
  page.on("pageerror", e => console.error("PAGE ERROR:", e.message));
  page.on("console", m => { if (m.type() === "error") console.error("CONSOLE:", m.text()); });

  for (const [view, state, file] of SHOTS) {
    const url = `${HTML}?chrome=0&view=${view}&state=${state}`;
    await page.goto(url);
    await page.waitForTimeout(2600); // 等入场动画/计数器稳定
    await page.screenshot({ path: path.join(OUT, file) });
    console.log("shot:", file);
  }
  await browser.close();
})().catch(e => { console.error(e); process.exit(1); });
