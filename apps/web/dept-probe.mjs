import { chromium } from "@playwright/test";
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
const bad = [];
page.on("response", async (r) => {
  if (r.status() >= 400 && (r.url().includes("7788") || r.url().includes("/api/"))) {
    let body = "";
    try { body = (await r.text()).slice(0, 300); } catch {}
    bad.push({ status: r.status(), url: r.url(), body });
  }
});
await page.goto("http://127.0.0.1:9001/login", { waitUntil: "networkidle" });
await page.locator('input[autocomplete="username"]').fill("admin");
await page.locator('input[autocomplete="current-password"]').fill("123456");
await page.getByRole("button", { name: /登录/ }).click();
await page.waitForURL(u => !u.pathname.includes("/login"), { timeout: 25000 });
await page.goto("http://127.0.0.1:9001/opm/departments", { waitUntil: "networkidle" });
await page.waitForTimeout(2500);
const pageText = await page.locator(".q-page").innerText().catch(()=>"");
console.log("PAGE:", pageText.slice(0,250));
console.log("BAD:", JSON.stringify(bad, null, 2));
await page.screenshot({ path: "E:/project/FrameWork/docs/ui-verify/05-opm_departments-debug.png" });
await browser.close();
