import { chromium } from "@playwright/test";
const b = await chromium.launch({ headless: true });
const p = await b.newPage({ viewport: { width: 1600, height: 900 } });
const reqs = [];
const resps = [];
p.on("request", (r) => {
  if (r.url().includes("auth") || r.url().includes("login")) {
    reqs.push({ m: r.method(), u: r.url(), post: r.postData() });
  }
});
p.on("response", async (r) => {
  if (r.url().includes("auth") || r.url().includes("login")) {
    let body = "";
    try { body = (await r.text()).slice(0, 400); } catch {}
    resps.push({ s: r.status(), u: r.url(), body });
  }
});
p.on("pageerror", (e) => console.log("PE", e.message));
p.on("console", (m) => {
  if (m.type() === "error" || m.type() === "warning") console.log(m.type(), m.text().slice(0, 250));
});
await p.goto("http://127.0.0.1:9001/login", { waitUntil: "networkidle" });
await p.waitForTimeout(1000);
await p.locator('input[autocomplete="username"]').fill("admin");
await p.locator('input[autocomplete="current-password"]').fill("123456");
const btn = p.locator(".q-btn").filter({ hasText: "\u767b\u5f55" });
console.log("btnCount", await btn.count(), "text", await btn.first().innerText().catch(() => ""));
await btn.first().click();
await p.waitForTimeout(5000);
console.log("url", p.url());
console.log("REQS", JSON.stringify(reqs, null, 2));
console.log("RESPS", JSON.stringify(resps, null, 2));
const toast = await p.locator(".q-notification").allInnerTexts().catch(() => []);
console.log("toasts", toast);
const body = (await p.locator("body").innerText()).slice(0, 400);
console.log("body", body);
await p.screenshot({ path: "E:/project/FrameWork/docs/ui-verify/02-login-debug.png" });
await b.close();