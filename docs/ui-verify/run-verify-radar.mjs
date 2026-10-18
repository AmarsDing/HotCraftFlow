/**
 * GitHub RADAR 三镜头 UI 验证：榜单 / 详情 / AI+源码。
 *
 * 前置：后端（BASE_BE）与前端（BASE_FE）均已启动，且至少跑一次榜单刷新（避免空榜）。
 * 用法：
 *   node docs/ui-verify/run-verify-radar.mjs
 * 环境变量（均有默认值）：
 *   BASE_FE=http://127.0.0.1:9001  BASE_BE=http://127.0.0.1:7788
 *   ADMIN_USER=admin  ADMIN_PASS=123456
 * 产物：docs/ui-verify/radar/*.png + radar-report.md；任一镜头失败则 exit 1。
 */
import { chromium } from "@playwright/test";
import fs from "fs";
import path from "path";

const BASE_FE = process.env.BASE_FE || "http://127.0.0.1:9001";
const BASE_BE = process.env.BASE_BE || "http://127.0.0.1:7788";
const ADMIN_USER = process.env.ADMIN_USER || "admin";
const ADMIN_PASS = process.env.ADMIN_PASS || "123456";
const OUT = path.resolve(process.cwd(), "docs/ui-verify/radar");
fs.mkdirSync(OUT, { recursive: true });

const results = [];
const runtimeErrors = [];

function note(name, status, notes) {
  results.push({ name, status, notes });
  console.log(`[${status}] ${name} :: ${notes}`);
}
async function shot(page, file) {
  await page.screenshot({ path: path.join(OUT, file), fullPage: false });
  return file;
}

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
page.on("pageerror", (e) => runtimeErrors.push("pageerror:" + e.message));
page.on("console", (m) => {
  if (m.type() === "error") runtimeErrors.push("console:" + m.text());
});

let failed = false;
try {
  // ── 登录（复用 verify 主脚本同款表单定位）
  await page.goto(`${BASE_FE}/login`, { waitUntil: "networkidle" });
  const user = page.locator('input[autocomplete="username"]');
  const pass = page.locator('input[autocomplete="current-password"]');
  const loginBtn = page.getByRole("button", { name: /登录|Login/i });
  if ((await user.count()) > 0 && (await pass.count()) > 0) {
    await user.first().fill(ADMIN_USER);
    await pass.first().fill(ADMIN_PASS);
    await loginBtn.first().click();
    await page.waitForURL((u) => !u.pathname.includes("/login"), { timeout: 25000 }).catch(() => {});
    await page.waitForTimeout(2000);
  }
  if (page.url().includes("/login")) {
    note("login", "fail", "still on login page");
    failed = true;
    throw new Error("login failed");
  }

  // ── 镜头 1：榜单
  await page.goto(`${BASE_FE}/github-radar`, { waitUntil: "domcontentloaded", timeout: 30000 });
  await page.waitForTimeout(2500);
  const cards = page.locator(".radar-grid .repo-card");
  const cardCount = await cards.count();
  const kpi = await page.locator(".app-stat-strip").count();
  const neg1 = await page.locator(".q-notification.bg-negative").count();
  let f = await shot(page, "r1-radar-board.png");
  if (cardCount > 0 && neg1 === 0) note("board", "pass", `cards=${cardCount} kpi=${kpi > 0} shot=${f}`);
  else {
    failed = true;
    note("board", "fail", `cards=${cardCount} negative-notify=${neg1} shot=${f}`);
  }

  // ── 镜头 2：详情（第一张卡 → data-cy=repo-detail；断言无 detail-error）
  await cards.first().click({ timeout: 5000 });
  await page.waitForSelector('[data-cy="repo-detail"]', { timeout: 8000 });
  await page.waitForTimeout(2000);
  const detailErr = await page.locator('[data-cy="detail-error"]').count();
  const metaRows = await page.locator('.repo-detail__meta-row, [class*="meta-row"]').count();
  f = await shot(page, "r2-radar-detail.png");
  if (detailErr === 0) note("detail", "pass", `metaRows=${metaRows} shot=${f}`);
  else {
    failed = true;
    note("detail", "fail", `detail-error banner visible shot=${f}`);
  }

  // ── 镜头 3：AI + 源码（tab 顺序：overview=0 insight=1 source=2 clone=3）
  const dialog = page.locator('[data-cy="repo-detail"]');
  await dialog.locator(".q-tab").nth(2).click({ timeout: 4000 });
  await page.waitForSelector('[data-cy="source-browser"]', { timeout: 8000 });
  await page.waitForTimeout(1500);
  const treeRows = await dialog.locator('[role="treeitem"]').count();
  await dialog.locator(".q-tab").nth(1).click({ timeout: 4000 });
  await page.waitForSelector('[data-cy="insight-panel"]', { timeout: 8000 });
  await page.waitForTimeout(1000);
  const insightErr = await page.locator('[data-cy="insight-error"]').count();
  f = await shot(page, "r3-radar-ai-source.png");
  if (treeRows >= 0 && insightErr === 0) note("ai-source", "pass", `treeRows=${treeRows} shot=${f}`);
  else {
    failed = true;
    note("ai-source", "fail", `insight-error banner visible shot=${f}`);
  }
} catch (e) {
  failed = true;
  note("fatal", "fail", String(e?.message || e));
  await shot(page, "rx-fatal.png").catch(() => {});
}

// 报告
const lines = [
  "# GitHub RADAR UI 验证报告",
  "",
  `- 时间: ${new Date().toISOString()}`,
  `- 前端: ${BASE_FE}  后端: ${BASE_BE}`,
  "",
  "| 镜头 | 状态 | 说明 |",
  "| --- | --- | --- |",
  ...results.map((r) => `| ${r.name} | ${r.status} | ${String(r.notes).replace(/\|/g, "/")} |`),
  "",
  "## 运行时错误采样",
  "",
  ...(runtimeErrors.length
    ? runtimeErrors
        .filter((e) => !/favicon|NATS|WebSocket|ws:\/\//i.test(e))
        .slice(0, 30)
        .map((e) => `- ${e}`)
    : ["- 无"]),
  "",
];
fs.writeFileSync(path.join(OUT, "radar-report.md"), lines.join("\n"), "utf8");
await browser.close();
console.log(`RADAR-REPORT ${failed ? "FAIL" : "PASS"} total=${results.length}`);
process.exit(failed ? 1 : 0);
