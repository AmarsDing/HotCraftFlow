import { chromium } from "@playwright/test";
const b = await chromium.launch({ headless: true });
const p = await b.newPage({ viewport: { width: 1600, height: 900 } });
p.on("pageerror", (e) => console.log("PE", e.message));
p.on("console", (m) => { if (["error","warning"].includes(m.type())) console.log(m.type(), m.text().slice(0,300)); });
await p.goto("http://127.0.0.1:9001/login", { waitUntil: "networkidle" });
await p.locator('input[autocomplete="username"]').fill("admin");
await p.locator('input[autocomplete="current-password"]').fill("123456");
await p.locator(".q-btn").filter({ hasText: "\u767b\u5f55" }).first().click();
await p.waitForURL("**/dashboard", { timeout: 20000 });
for (const path of ["/opm/users", "/permission/menus", "/opm/organizations"]) {
  await p.goto("http://127.0.0.1:9001" + path, { waitUntil: "domcontentloaded" });
  await p.waitForTimeout(2000);
  const info = await p.evaluate(() => ({
    body: (document.body.innerText||"").trim().slice(0,400),
    qpage: document.querySelectorAll(".q-page").length,
    err: !!document.querySelector(".err-boundary"),
    errMsg: document.querySelector(".err-boundary__msg")?.textContent || "",
    host: !!document.querySelector(".err-boundary-host"),
    appChildren: document.querySelector("#q-app")?.childElementCount || 0,
  }));
  console.log("PATH", path, JSON.stringify(info));
}
await b.close();