import { chromium } from "@playwright/test";
const b = await chromium.launch({ headless: true });
const p = await b.newPage({ viewport: { width: 1600, height: 900 } });
p.on("response", async (r) => {
  if (r.status() >= 400) {
    let body = "";
    try { body = (await r.text()).slice(0, 400); } catch {}
    console.log("HTTP", r.status(), r.request().method(), r.url(), body.slice(0, 200));
  }
});
p.on("pageerror", (e) => console.log("PE", e.message));
p.on("console", (m) => { if (m.type() === "error") console.log("CE", m.text().slice(0, 300)); });
await p.goto("http://127.0.0.1:9001/login", { waitUntil: "networkidle" });
await p.locator('input[autocomplete="username"]').fill("admin");
await p.locator('input[autocomplete="current-password"]').fill("123456");
await p.locator(".q-btn").filter({ hasText: "\u767b\u5f55" }).first().click();
await p.waitForTimeout(6000);
console.log("URL", p.url());
const ls = await p.evaluate(() => ({
  at: localStorage.getItem("framework_access_token")?.slice(0, 20),
  rt: localStorage.getItem("framework_refresh_token")?.slice(0, 20),
  keys: Object.keys(localStorage),
}));
console.log("LS", ls);
await b.close();