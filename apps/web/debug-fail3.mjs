import { chromium } from "@playwright/test";
const b = await chromium.launch({ headless: true });
const p = await b.newPage({ viewport: { width: 1600, height: 900 } });
const bad = [];
p.on("response", async (r) => {
  if (r.status() >= 400) {
    let body = "";
    try { body = (await r.text()).slice(0, 300); } catch {}
    bad.push({ s: r.status(), u: r.url(), body });
  }
});
p.on("pageerror", (e) => console.log("PE", e.message));
p.on("console", (m) => { if (m.type() === "error") console.log("CE", m.text().slice(0, 250)); });
await p.goto("http://127.0.0.1:9001/login", { waitUntil: "networkidle" });
await p.locator('input[autocomplete="username"]').fill("admin");
await p.locator('input[autocomplete="current-password"]').fill("123456");
await p.locator(".q-btn").filter({ hasText: "\u767b\u5f55" }).first().click();
await p.waitForURL("**/dashboard", { timeout: 20000 });
for (const path of ["/opm/users", "/permission/menus", "/system/configs"]) {
  bad.length = 0;
  await p.goto("http://127.0.0.1:9001" + path, { waitUntil: "networkidle", timeout: 30000 }).catch(() => {});
  await p.waitForTimeout(2500);
  const text = ((await p.locator(".q-page, #q-app").first().innerText().catch(() => "")) || "").trim();
  console.log("\nPATH", path, "url", p.url(), "len", text.length);
  console.log("TEXT", JSON.stringify(text.slice(0, 300)));
  console.log("BAD", JSON.stringify(bad.slice(0, 5)));
  await p.screenshot({ path: `E:/project/FrameWork/docs/ui-verify/fail-${path.replace(/\//g, "_")}.png` });
}
await b.close();