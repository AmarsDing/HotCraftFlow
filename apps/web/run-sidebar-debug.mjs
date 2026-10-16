import { chromium } from '@playwright/test';
import fs from 'fs';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.goto('http://localhost:9001/login', { waitUntil: 'networkidle' });
if (await page.locator('input[type="password"]').count()) {
  await page.locator('input:not([type="password"]):not([type="hidden"])').first().fill('admin');
  await page.locator('input[type="password"]').fill('123456');
  await page.locator('button:has-text("登录"), .q-btn:has-text("登录")').first().click();
  await page.waitForTimeout(3000);
}
await page.goto('http://localhost:9001/dashboard', { waitUntil: 'networkidle' });
await page.waitForTimeout(1500);
// try expand drawer
const expand = page.locator('[aria-label*="展开"], button:has-text("menu"), .app-header-icon-btn').first();
if (await expand.count()) { try { await expand.click({timeout:2000}); await page.waitForTimeout(500);} catch{} }
const info = await page.evaluate(() => {
  const drawers = [...document.querySelectorAll('.q-drawer, .app-sidebar, aside')].map(el => ({
    cls: el.className,
    text: (el.innerText||'').slice(0,500),
    htmlLen: el.innerHTML.length,
  }));
  const items = [...document.querySelectorAll('.q-item, .app-sidebar-section-label__text')].map(el => (el.innerText||el.textContent||'').trim()).filter(Boolean).slice(0,40);
  return { drawers, items, bodyHasNav: /nav\.|NAV\./.test(document.body.innerText), bodySample: document.body.innerText.slice(0,800) };
});
fs.writeFileSync('E:/project/FrameWork/docs/ui-verify/retest-sidebar-debug.json', JSON.stringify(info,null,2));
await page.screenshot({ path: 'E:/project/FrameWork/docs/ui-verify/retest-sidebar2.png', fullPage: true });
console.log(JSON.stringify(info, null, 2));
await browser.close();
