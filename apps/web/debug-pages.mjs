import { chromium } from "@playwright/test";
const b = await chromium.launch({ headless: true });
const p = await b.newPage({ viewport: { width: 1600, height: 900 } });
const bad = [];
p.on("response", async (r) => {
  if (r.status() >= 400) {
    let body = "";
    try { body = (await r.text()).slice(0, 250); } catch {}
    bad.push({ s: r.status(), u: r.url(), body });
  }
});
p.on("pageerror", (e) => console.log("PE", e.message));
await p.goto("http://127.0.0.1:9001/login", { waitUntil: "networkidle" });
await p.locator('input[autocomplete="username"]').fill("admin");
await p.locator('input[autocomplete="current-password"]').fill("123456");
await p.locator(".q-btn").filter({ hasText: "\u767b\u5f55" }).first().click();
await p.waitForURL("**/dashboard", { timeout: 20000 });
await p.waitForTimeout(1000);
for (const path of ["/opm/organizations", "/opm/users", "/permission/menus", "/system/dicts", "/system/online-users", "/tenant", "/log/login"]) {
  bad.length = 0;
  await p.goto("http://127.0.0.1:9001" + path, { waitUntil: "domcontentloaded" });
  await p.waitForTimeout(2500);
  const text = ((await p.locator(".q-page").first().innerText().catch(() => "")) || "").trim().slice(0, 200);
  const layoutText = ((await p.locator(".q-layout, #q-app").first().innerText().catch(() => "")) || "").trim().slice(0, 200);
  console.log("PATH", path, "url", p.url(), "pageLen", text.length, "layoutLen", layoutText.length);
  console.log("PAGE_TEXT", JSON.stringify(text));
  console.log("BAD", JSON.stringify(bad.slice(0, 3)));
  await p.screenshot({ path: `E:/project/FrameWork/docs/ui-verify/dbg-${path.replace(/\//g,"_")}.png` });
}
await b.close();