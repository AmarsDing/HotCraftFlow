import { chromium } from "@playwright/test";
import fs from "fs";
import path from "path";
const OUT = "E:/project/FrameWork/docs/ui-verify";
fs.mkdirSync(OUT, { recursive: true });
const PAGES = [
  { name: "dashboard", path: "/dashboard" },
  { name: "organizations", path: "/opm/organizations", crud: true },
  { name: "departments", path: "/opm/departments", crud: true },
  { name: "positions", path: "/opm/positions", crud: true },
  { name: "users", path: "/opm/users", crud: true },
  { name: "roles", path: "/permission/roles", crud: true },
  { name: "menus", path: "/permission/menus", crud: true },
  { name: "dicts", path: "/system/dicts", crud: true },
  { name: "files", path: "/system/files", crud: true },
  { name: "tasks", path: "/system/tasks", crud: true },
  { name: "api-resources", path: "/system/api-resources", crud: true },
  { name: "configs", path: "/system/configs", crud: true },
  { name: "login-restrictions", path: "/system/login-restrictions", crud: true },
  { name: "online-users", path: "/system/online-users" },
  { name: "tenant", path: "/tenant", crud: true },
  { name: "login-log", path: "/log/login" },
  { name: "operation-log", path: "/log/operation" },
];
const ZH = {
  dashboard: "\u4eea\u8868\u76d8", organizations: "\u7ec4\u7ec7", departments: "\u90e8\u95e8",
  positions: "\u5c97\u4f4d", users: "\u7528\u6237", roles: "\u89d2\u8272", menus: "\u83dc\u5355",
  dicts: "\u5b57\u5178", files: "\u6587\u4ef6", tasks: "\u4efb\u52a1", "api-resources": "API\u8d44\u6e90",
  configs: "\u7cfb\u7edf\u914d\u7f6e", "login-restrictions": "\u767b\u5f55\u9650\u5236",
  "online-users": "\u5728\u7ebf\u7528\u6237", tenant: "\u79df\u6237", "login-log": "\u767b\u5f55\u65e5\u5fd7",
  "operation-log": "\u64cd\u4f5c\u65e5\u5fd7", "login-api": "\u767b\u5f55API", "login-page": "\u767b\u5f55\u9875",
  "login-submit": "\u767b\u5f55\u63d0\u4ea4",
};
const NEW_RE = /\u65b0\u5efa|\u521b\u5efa|\u65b0\u589e/;
const CANCEL_RE = /\u53d6\u6d88|\u5173\u95ed/;
const LOGIN_RE = /\u767b\u5f55/;
const NF_RE = /404|Not Found|\u9875\u9762\u4e0d\u5b58\u5728/;
const results = [];
const runtimeErrors = [];
function note(name, pth, status, notes, shot) {
  results.push({ name, path: pth, status, notes, shot });
  console.log(`[${status}] ${name} ${pth} :: ${notes}`);
}
async function shot(page, file) {
  await page.screenshot({ path: path.join(OUT, file), fullPage: false });
  return file;
}
async function tryOpenCreate(page) {
  const candidates = [
    page.getByRole("button", { name: NEW_RE }),
    page.locator(".q-btn").filter({ hasText: "\u65b0\u5efa" }),
    page.locator("button").filter({ hasText: "\u65b0\u5efa" }),
  ];
  for (const c of candidates) {
    try {
      if ((await c.count()) > 0 && (await c.first().isVisible())) {
        await c.first().click({ timeout: 3000 });
        await page.waitForTimeout(700);
        const dialog = page.locator('.q-dialog, .q-drawer--right, [role="dialog"], .q-drawer');
        if ((await dialog.count()) > 0 && (await dialog.first().isVisible().catch(() => false))) {
          const cancel = page.getByRole("button", { name: CANCEL_RE });
          if ((await cancel.count()) > 0) await cancel.first().click({ timeout: 2000 }).catch(() => {});
          else await page.keyboard.press("Escape");
          await page.waitForTimeout(300);
          return "create-dialog-ok";
        }
        await page.keyboard.press("Escape");
        return "create-clicked-no-dialog";
      }
    } catch {}
  }
  return "no-create-btn";
}
function writeReport() {
  const pass = results.filter((r) => r.status === "pass").length;
  const fail = results.filter((r) => r.status === "fail").length;
  const blocked = results.filter((r) => r.status === "blocked").length;
  const lines = [];
  lines.push("# FrameWork UI \u9a8c\u8bc1\u62a5\u544a");
  lines.push("");
  lines.push(`- \u65f6\u95f4: ${new Date().toISOString()} (UTC; Asia/Shanghai UTC+8)`);
  lines.push("- \u524d\u7aef: http://127.0.0.1:9001/");
  lines.push("- \u540e\u7aef: http://127.0.0.1:7788");
  lines.push("- \u8d26\u53f7: admin");
  lines.push(`- \u6c47\u603b: \u901a\u8fc7 ${pass} / \u5931\u8d25 ${fail} / \u963b\u585e ${blocked} / \u5171 ${results.length}`);
  lines.push("");
  lines.push("## \u542f\u52a8\u963b\u585e\u4fee\u590d\uff08\u9a8c\u8bc1\u8fc7\u7a0b\u4e2d\uff09");
  lines.push("");
  lines.push("1. `componentMap.ts` \u8865 `resolveComponent` \u522b\u540d");
  lines.push("2. `services/index.ts` \u8865 `myNoticeService` stub");
  lines.push("3. `BlankLayout.vue` \u59cb\u7ec8\u6e32\u67d3\u5b50\u8def\u7531");
  lines.push("4. \u767b\u5f55\u8def\u7531\u8865 `meta.layout: blank`");
  lines.push("");
  lines.push("## \u7ed3\u679c\u660e\u7ec6");
  lines.push("");
  lines.push("| \u529f\u80fd | \u8def\u5f84 | \u72b6\u6001 | \u8bf4\u660e | \u622a\u56fe |");
  lines.push("| --- | --- | --- | --- | --- |");
  for (const r of results) {
    const st = r.status === "pass" ? "\u901a\u8fc7" : r.status === "fail" ? "\u5931\u8d25" : "\u963b\u585e";
    const zh = ZH[r.name] || r.name;
    lines.push(`| ${zh} | \`${r.path}\` | ${st} | ${(r.notes || "").replace(/\|/g, "/")} | ${r.shot || "-"} |`);
  }
  lines.push("");
  lines.push("## \u5931\u8d25\u9879");
  lines.push("");
  const fails = results.filter((r) => r.status !== "pass");
  if (!fails.length) lines.push("\u65e0");
  else fails.forEach((f) => lines.push(`- **${ZH[f.name] || f.name}** (\`${f.path}\`): ${f.notes}`));
  lines.push("");
  lines.push("## \u63a7\u5236\u53f0\u9519\u8bef\u91c7\u6837");
  lines.push("");
  const filtered = runtimeErrors.filter((e) => !/favicon|NATS|WebSocket|ws:\/\//i.test(e));
  if (!filtered.length) lines.push("\u65e0\u663e\u8457\u9519\u8bef");
  else filtered.slice(0, 50).forEach((e) => lines.push(`- ${e}`));
  lines.push("");
  lines.push("## \u622a\u56fe\u76ee\u5f55");
  lines.push("");
  lines.push("`E:\\\\project\\\\FrameWork\\\\docs\\\\ui-verify\\\\`");
  lines.push("");
  fs.writeFileSync(path.join(OUT, "report.md"), lines.join("\n"), "utf8");
  fs.writeFileSync(path.join(OUT, "results.json"), JSON.stringify({ results, runtimeErrors }, null, 2), "utf8");
  console.log(`REPORT pass=${pass} fail=${fail} total=${results.length}`);
}
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
page.on("pageerror", (e) => runtimeErrors.push("pageerror:" + e.message));
page.on("console", (m) => { if (m.type() === "error") runtimeErrors.push("console:" + m.text()); });
try {
  const r = await page.request.post("http://127.0.0.1:7788/api/v1/monitor/auth/login", {
    data: { grant_type: "password", username: "admin", password: "123456" },
  });
  const j = await r.json();
  note("login-api", "/api/v1/monitor/auth/login", r.ok() && j.access_token ? "pass" : "fail", `http=${r.status()} token=${!!j.access_token}`);
} catch (e) {
  note("login-api", "/api/v1/monitor/auth/login", "fail", String(e));
}
await page.goto("http://127.0.0.1:9001/login", { waitUntil: "networkidle" });
await page.waitForTimeout(1000);
let f = await shot(page, "01-login.png");
const user = page.locator('input[autocomplete="username"]');
const pass = page.locator('input[autocomplete="current-password"]');
const loginBtn = page.getByRole("button", { name: LOGIN_RE });
const loginOk = (await user.count()) > 0 && (await pass.count()) > 0 && (await loginBtn.count()) > 0;
note("login-page", "/login", loginOk ? "pass" : "fail", loginOk ? "form visible" : "missing controls", f);
if (!loginOk) { writeReport(); await browser.close(); process.exit(1); }
await user.first().fill("admin");
await pass.first().fill("123456");
await loginBtn.first().click();
await page.waitForURL((u) => !u.pathname.includes("/login"), { timeout: 25000 }).catch(() => {});
await page.waitForTimeout(2500);
f = await shot(page, "02-after-login.png");
const loggedIn = !page.url().includes("/login");
note("login-submit", page.url(), loggedIn ? "pass" : "fail", loggedIn ? "entered app" : "still on login", f);
if (!loggedIn) { writeReport(); await browser.close(); process.exit(1); }
let idx = 3;
for (const item of PAGES) {
  const file = `${String(idx).padStart(2, "0")}-${item.path.replace(/\//g, "_").replace(/^_/, "")}.png`;
  idx++;
  const before = runtimeErrors.length;
  try {
    await page.goto("http://127.0.0.1:9001" + item.path, { waitUntil: "domcontentloaded", timeout: 30000 });
    await page.waitForTimeout(1800);
    const shotName = await shot(page, file);
    const notFound = (await page.getByText(NF_RE).count()) > 0;
    const negative = page.locator(".q-notification.bg-negative");
    const hasNeg = (await negative.count()) > 0 && (await negative.first().isVisible().catch(() => false));
    const bodyText = ((await page.locator(".q-page, #q-app").first().innerText().catch(() => "")) || "").trim();
    const blank = bodyText.length < 8;
    const hasTable = (await page.locator("table, .q-table, .q-tree, .q-list, .q-virtual-scroll").count()) > 0;
    let crud = "";
    if (item.crud) {
      crud = await tryOpenCreate(page);
      if (crud === "create-dialog-ok") await shot(page, file.replace(".png", "-create.png"));
    }
    const newErr = runtimeErrors.slice(before).filter((e) => !/favicon|NATS|WebSocket|ws:\/\//i.test(e));
    const severe = newErr.some((e) => /TypeError|ReferenceError|Cannot read|Failed to fetch| 500| 401| 403/i.test(e));
    let status = "pass";
    const notes = [];
    notes.push(hasTable || bodyText.length > 20 ? "content-ok" : "content-thin");
    if (crud) notes.push("CRUD:" + crud);
    if (hasNeg) { status = "fail"; notes.push("negative-notify"); }
    if (notFound) { status = "fail"; notes.push("maybe-404"); }
    if (blank) { status = "fail"; notes.push("blank"); }
    if (severe) { status = "fail"; notes.push("console:" + newErr.slice(0, 2).join(";").slice(0, 160)); }
    else if (newErr.length) notes.push("console-hint:" + newErr.slice(0, 1).join(";").slice(0, 120));
    note(item.name, item.path, status, notes.join(" | "), shotName);
  } catch (e) {
    await shot(page, file).catch(() => {});
    note(item.name, item.path, "fail", "open-failed:" + (e?.message || e), file);
  }
}
writeReport();
await browser.close();
