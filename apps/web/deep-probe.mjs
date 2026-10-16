import { chromium } from "@playwright/test";
import fs from "fs";
const OUT = "E:/project/FrameWork/docs/ui-verify";
const paths = ["/dashboard","/opm/users","/permission/menus","/system/dicts","/opm/organizations"];
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
const logs = [];
page.on("pageerror", e => logs.push("PE:"+e.message));
page.on("console", m => { if (m.type()==="error") logs.push("CE:"+m.text()); });
page.on("response", r => { if (r.status()>=400 && r.url().includes("7788")) logs.push("HTTP"+r.status()+" "+r.url()); });
await page.goto("http://127.0.0.1:9001/login", { waitUntil: "networkidle" });
await page.locator('input[autocomplete="username"]').fill("admin");
await page.locator('input[autocomplete="current-password"]').fill("123456");
await page.getByRole("button", { name: /登录/ }).click();
await page.waitForURL(u => !u.pathname.includes("/login"), { timeout: 25000 });
await page.waitForTimeout(2000);
const report = [];
for (const p of paths) {
  logs.length = 0;
  await page.goto("http://127.0.0.1:9001"+p, { waitUntil: "networkidle", timeout: 30000 }).catch(e=>logs.push("NAV:"+e.message));
  await page.waitForTimeout(2500);
  const info = await page.evaluate(() => {
    const pageEl = document.querySelector(".q-page");
    const main = document.querySelector(".q-page-container, .q-layout__section--marginal + div, main, .q-page");
    const layout = document.querySelector(".q-layout");
    const drawerText = (document.querySelector(".q-drawer")?.innerText||"").trim().slice(0,120);
    const pageText = (pageEl?.innerText||"").trim();
    const children = pageEl ? pageEl.children.length : -1;
    const htmlLen = pageEl ? pageEl.innerHTML.length : 0;
    return {
      url: location.href,
      hasLayout: !!layout,
      hasPage: !!pageEl,
      pageChildren: children,
      pageHtmlLen: htmlLen,
      pageTextLen: pageText.length,
      pageText: pageText.slice(0,200),
      drawerSample: drawerText,
      title: document.title,
    };
  });
  const shot = p.replace(/\//g,"_").replace(/^_/,"") + "-deep.png";
  await page.screenshot({ path: OUT + "/" + shot, fullPage: false });
  report.push({ path: p, ...info, logs: [...logs], shot });
}
fs.writeFileSync(OUT + "/deep-probe.json", JSON.stringify(report, null, 2), "utf8");
console.log(JSON.stringify(report, null, 2));
await browser.close();
