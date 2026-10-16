import { chromium } from '@playwright/test';
import fs from 'fs';
import path from 'path';

const BASE = process.env.UI_BASE || 'http://localhost:9001';
const OUT = 'E:/project/FrameWork/docs/ui-verify';
fs.mkdirSync(OUT, { recursive: true });

const results = [];
function ok(name, detail='') { results.push({name, pass:true, detail}); console.log('PASS', name, detail); }
function fail(name, detail='') { results.push({name, pass:false, detail}); console.log('FAIL', name, detail); }

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
page.setDefaultTimeout(20000);

try {
  await page.goto(BASE + '/login', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(800);
  // fill login
  const user = page.locator('input[type="text"], input[name="username"], input').first();
  await page.locator('input').nth(0).fill('admin');
  await page.locator('input[type="password"]').fill('123456');
  await page.getByRole('button', { name: /登\s*录|Login|SIGN/i }).first().click();
  await page.waitForTimeout(2500);
  const url = page.url();
  if (/login/i.test(url) && !(await page.locator('.app-sidebar, .q-drawer').count())) {
    fail('login', url);
  } else {
    ok('login', url);
  }

  // sidebar labels should not show raw nav.* keys
  const sidebarText = await page.locator('.q-drawer, .app-sidebar').innerText().catch(()=> '');
  const rawKeys = [...sidebarText.matchAll(/\b(?:nav|NAV)\.[A-Za-z0-9_.]+\b/g)].map(m=>m[0]);
  if (rawKeys.length) fail('sidebar-i18n', rawKeys.join(','));
  else ok('sidebar-i18n', sidebarText.split(/\n/).filter(Boolean).slice(0,12).join(' | '));

  // expect Chinese labels present
  const need = ['用户','组织','部门','仪表盘'];
  const missing = need.filter(n => !sidebarText.includes(n));
  if (missing.length) fail('sidebar-zh', 'missing '+missing.join(','));
  else ok('sidebar-zh', 'found '+need.join(','));

  await page.screenshot({ path: path.join(OUT, 'retest-sidebar.png'), fullPage: true });

  // departments page
  await page.goto(BASE + '/opm/departments', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2000);
  const usersReq = [];
  page.on('response', async (res) => {
    if (res.url().includes('/api/v1/monitor/users')) {
      usersReq.push({ status: res.status(), url: res.url() });
    }
  });
  // open create dialog
  const createBtn = page.getByRole('button', { name: /新建|创建|新增/ }).first();
  if (await createBtn.count()) {
    await createBtn.click();
    await page.waitForTimeout(2000);
  } else {
    // try icon/add
    const add = page.locator('button:has-text("新建"), .q-btn:has-text("新建")').first();
    if (await add.count()) { await add.click(); await page.waitForTimeout(2000); }
  }

  // wait a bit for users request
  await page.waitForTimeout(1500);
  const bad = usersReq.filter(r => r.status >= 400);
  if (bad.length) fail('dept-users-api', JSON.stringify(bad));
  else if (usersReq.length === 0) {
    // navigate again to trigger
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(1500);
    if (await createBtn.count()) { await createBtn.click(); await page.waitForTimeout(2000); }
  }
  const usersReq2 = usersReq;
  const bad2 = usersReq2.filter(r => r.status >= 400);
  if (bad2.length) fail('dept-users-api', JSON.stringify(bad2));
  else ok('dept-users-api', JSON.stringify(usersReq2.slice(-3)));

  // leader dropdown should not be empty-only error; check dialog content
  const dialog = page.locator('.q-dialog, .q-dialog__inner').first();
  const dialogText = await dialog.innerText().catch(()=> '');
  await page.screenshot({ path: path.join(OUT, 'retest-dept-create.png'), fullPage: true });
  if (/500|query list failed|Internal/i.test(dialogText)) fail('dept-dialog', dialogText.slice(0,200));
  else ok('dept-dialog', dialogText.split(/\n/).filter(Boolean).slice(0,8).join(' | '));

} catch (e) {
  fail('exception', String(e));
  await page.screenshot({ path: path.join(OUT, 'retest-error.png'), fullPage: true }).catch(()=>{});
} finally {
  await browser.close();
}

const report = [
  '# UI Retest ' + new Date().toISOString(),
  '',
  ...results.map(r => `- ${r.pass?'PASS':'FAIL'} **${r.name}**: ${r.detail}`),
  '',
  `Summary: ${results.filter(r=>r.pass).length}/${results.length} passed`,
].join('\n');
fs.writeFileSync(path.join(OUT, 'retest-report.md'), report, 'utf8');
console.log(report);
process.exit(results.every(r=>r.pass)?0:1);

