import { chromium } from "@playwright/test";
const b = await chromium.launch({ headless: true });
const p = await b.newPage({ viewport: { width: 1600, height: 900 } });
const api = [];
p.on("response", async (r) => {
  const u = r.url();
  if (!u.includes("/api/")) return;
  let body = "";
  try { body = (await r.text()).slice(0, 500); } catch {}
  api.push({ s: r.status(), m: r.request().method(), u, body });
});
p.on("console", (m) => {
  if (m.type() === "error") console.log("CE", m.text().slice(0, 300));
});
p.on("pageerror", (e) => console.log("PE", e.message));
await p.goto("http://127.0.0.1:9001/login", { waitUntil: "networkidle" });
await p.locator('input[autocomplete="username"]').fill("admin");
await p.locator('input[autocomplete="current-password"]').fill("123456");
await p.locator(".q-btn").filter({ hasText: "\u767b\u5f55" }).first().click();
await p.waitForTimeout(8000);
console.log("FINAL_URL", p.url());
console.log("API_CALLS");
for (const a of api) console.log(JSON.stringify(a));
await p.screenshot({ path: "E:/project/FrameWork/docs/ui-verify/02-after-login-debug.png" });
await b.close();