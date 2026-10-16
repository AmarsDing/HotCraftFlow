import { test, expect, Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

const OUT = 'E:/project/FrameWork/docs/ui-verify';
const SHOTS = OUT;

type Result = {
  name: string;
  path: string;
  status: 'pass' | 'fail' | 'blocked';
  notes: string;
  shot?: string;
};

const results: Result[] = [];

const PAGES: { name: string; path: string; crud?: boolean }[] = [
  { name: '仪表盘', path: '/dashboard' },
  { name: '组织', path: '/opm/organizations', crud: true },
  { name: '部门', path: '/opm/departments', crud: true },
  { name: '岗位', path: '/opm/positions', crud: true },
  { name: '用户', path: '/opm/users', crud: true },
  { name: '角色', path: '/permission/roles', crud: true },
  { name: '菜单', path: '/permission/menus', crud: true },
  { name: '字典', path: '/system/dicts', crud: true },
  { name: '文件', path: '/system/files', crud: true },
  { name: '任务', path: '/system/tasks', crud: true },
  { name: 'API资源', path: '/system/api-resources', crud: true },
  { name: '系统配置', path: '/system/configs', crud: true },
  { name: '登录限制', path: '/system/login-restrictions', crud: true },
  { name: '在线用户', path: '/system/online-users' },
  { name: '租户', path: '/tenant', crud: true },
  { name: '登录日志', path: '/log/login' },
  { name: '操作日志', path: '/log/operation' },
];

function ensureDir() {
  fs.mkdirSync(SHOTS, { recursive: true });
}

async function collectPageErrors(page: Page) {
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(`pageerror: ${e.message}`));
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(`console: ${msg.text()}`);
  });
  return errors;
}

async function shot(page: Page, file: string) {
  const p = path.join(SHOTS, file);
  await page.screenshot({ path: p, fullPage: false });
  return file;
}

async function hasVisibleErrorBanner(page: Page) {
  // Quasar negative notify / obvious error text
  const negative = page.locator('.q-notification.bg-negative, .q-banner.bg-negative, .text-negative');
  if (await negative.count()) {
    const visible = await negative.first().isVisible().catch(() => false);
    if (visible) return await negative.first().innerText().catch(() => 'negative banner');
  }
  return '';
}

async function pageLooksBlank(page: Page) {
  const main = page.locator('.q-page, main, #q-app');
  const text = ((await main.first().innerText().catch(() => '')) || '').trim();
  // very short content after load may indicate blank
  return text.length < 8;
}

async function tryOpenCreate(page: Page) {
  const candidates = [
    page.getByRole('button', { name: /新建|创建|新增|Create/i }),
    page.locator('button:has-text("新建")'),
    page.locator('.q-btn:has-text("新建")'),
    page.locator('button[aria-label*="新建"], button[aria-label*="create" i]'),
    page.locator('.q-btn:has(i:text("add"))').first(),
  ];
  for (const c of candidates) {
    try {
      if ((await c.count()) > 0 && (await c.first().isVisible())) {
        await c.first().click({ timeout: 3000 });
        await page.waitForTimeout(600);
        const dialog = page.locator('.q-dialog, .q-drawer--right, .form-drawer, [role="dialog"]');
        if ((await dialog.count()) > 0 && (await dialog.first().isVisible().catch(() => false))) {
          // close without saving
          const cancel = page.getByRole('button', { name: /取消|关闭|Cancel|Close/i });
          if ((await cancel.count()) > 0) {
            await cancel.first().click({ timeout: 2000 }).catch(() => {});
          } else {
            await page.keyboard.press('Escape');
          }
          await page.waitForTimeout(300);
          return 'create-dialog-ok';
        }
        await page.keyboard.press('Escape');
        return 'create-clicked-no-dialog';
      }
    } catch {
      /* try next */
    }
  }
  return 'no-create-btn';
}

test.describe.configure({ mode: 'serial' });

test('FrameWork UI verify', async ({ page }) => {
  ensureDir();
  const runtimeErrors = await collectPageErrors(page);

  // --- Login page ---
  await page.goto('/login', { waitUntil: 'networkidle' });
  await page.waitForTimeout(800);
  const loginShot = await shot(page, '01-login.png');
  const userInput = page.locator('input[autocomplete="username"]');
  const passInput = page.locator('input[autocomplete="current-password"]');
  const loginOk =
    (await userInput.count()) > 0 &&
    (await passInput.count()) > 0 &&
    (await page.getByRole('button', { name: /登录|Login|Submit/i }).count()) > 0;
  results.push({
    name: '登录页',
    path: '/login',
    status: loginOk ? 'pass' : 'fail',
    notes: loginOk ? '表单控件可见' : '缺少用户名/密码/提交控件',
    shot: loginShot,
  });
  expect(loginOk).toBeTruthy();

  // Fill & submit
  await userInput.first().fill('admin');
  await passInput.first().fill('123456');
  await page.getByRole('button', { name: /登录|Login/i }).first().click();
  await page.waitForURL((u) => !u.pathname.includes('/login'), { timeout: 20000 }).catch(() => {});
  await page.waitForTimeout(1500);

  const afterLogin = page.url();
  const loggedIn = !afterLogin.includes('/login');
  const dashShot = await shot(page, '02-after-login.png');
  results.push({
    name: '登录提交',
    path: afterLogin,
    status: loggedIn ? 'pass' : 'fail',
    notes: loggedIn ? `跳转到 ${afterLogin}` : `仍停留登录页；errors=${runtimeErrors.slice(-3).join(' | ')}`,
    shot: dashShot,
  });
  if (!loggedIn) {
    writeReport(results, runtimeErrors);
    throw new Error('login failed, abort remaining checks');
  }

  // Visit each page
  let idx = 3;
  for (const item of PAGES) {
    const file = `${String(idx).padStart(2, '0')}-${item.path.replace(/\//g, '_').replace(/^_/, '') || 'root'}.png`;
    idx += 1;
    const beforeErr = runtimeErrors.length;
    try {
      const respPromise = page.waitForResponse(
        (r) => r.url().includes('/api/') && r.request().method() === 'GET',
        { timeout: 8000 },
      ).catch(() => null);

      await page.goto(item.path, { waitUntil: 'domcontentloaded', timeout: 30000 });
      await page.waitForTimeout(1200);
      await respPromise;
      await page.waitForTimeout(800);

      const shotName = await shot(page, file);
      const banner = await hasVisibleErrorBanner(page);
      const blank = await pageLooksBlank(page);
      const notFound = (await page.locator('text=/404|Not Found|页面不存在/i').count()) > 0;
      const newErrors = runtimeErrors.slice(beforeErr).filter((e) => !e.includes('favicon') && !e.includes('NATS') && !e.includes('WebSocket') && !e.includes('ws://'));

      let crudNote = '';
      if (item.crud) {
        crudNote = await tryOpenCreate(page);
        // re-shot after create attempt if dialog was opened
        if (crudNote === 'create-dialog-ok') {
          await shot(page, file.replace('.png', '-create.png'));
        }
      }

      // list loaded heuristic: table/rows or empty state or any substantial text
      const hasTable = (await page.locator('table, .q-table, .q-virtual-scroll, .q-tree, .q-list').count()) > 0;
      const bodyText = ((await page.locator('.q-page').first().innerText().catch(() => '')) || '').trim();
      const listHint = hasTable || bodyText.length > 20 ? '列表/内容已渲染' : '内容偏少';

      let status: Result['status'] = 'pass';
      const notesParts = [listHint];
      if (crudNote) notesParts.push(`CRUD:${crudNote}`);
      if (banner) {
        status = 'fail';
        notesParts.push(`红错:${banner.slice(0, 80)}`);
      }
      if (notFound) {
        status = 'fail';
        notesParts.push('疑似404');
      }
      if (blank) {
        status = 'fail';
        notesParts.push('页面疑似空白');
      }
      if (newErrors.length) {
        // console errors alone -> fail if severe, else note
        const severe = newErrors.some((e) => /TypeError|ReferenceError|Cannot read|Failed to fetch|500|401|403/i.test(e));
        if (severe) status = 'fail';
        notesParts.push(`控制台:${newErrors.slice(0, 2).join(' ; ').slice(0, 160)}`);
      }

      results.push({
        name: item.name,
        path: item.path,
        status,
        notes: notesParts.join(' | '),
        shot: shotName,
      });
    } catch (e: any) {
      await shot(page, file).catch(() => '');
      results.push({
        name: item.name,
        path: item.path,
        status: 'fail',
        notes: `打开失败: ${e?.message || e}`,
        shot: file,
      });
    }
  }

  writeReport(results, runtimeErrors);

  const fails = results.filter((r) => r.status === 'fail');
  expect(fails, fails.map((f) => `${f.name}:${f.notes}`).join('\n')).toHaveLength(0);
});

function writeReport(rows: Result[], runtimeErrors: string[]) {
  ensureDir();
  const pass = rows.filter((r) => r.status === 'pass').length;
  const fail = rows.filter((r) => r.status === 'fail').length;
  const blocked = rows.filter((r) => r.status === 'blocked').length;
  const lines: string[] = [];
  lines.push('# FrameWork UI 验证报告');
  lines.push('');
  lines.push(`- 时间: ${new Date().toISOString()} (UTC)`);
  lines.push(`- 前端: http://127.0.0.1:9001/`);
  lines.push(`- 后端: http://127.0.0.1:7788`);
  lines.push(`- 账号: admin`);
  lines.push(`- 汇总: 通过 ${pass} / 失败 ${fail} / 阻塞 ${blocked} / 共 ${rows.length}`);
  lines.push('');
  lines.push('## 结果明细');
  lines.push('');
  lines.push('| 功能 | 路径 | 状态 | 说明 | 截图 |');
  lines.push('| --- | --- | --- | --- | --- |');
  for (const r of rows) {
    const st = r.status === 'pass' ? '通过' : r.status === 'fail' ? '失败' : '阻塞';
    lines.push(`| ${r.name} | \`${r.path}\` | ${st} | ${r.notes.replace(/\|/g, '/')} | ${r.shot || '-'} |`);
  }
  lines.push('');
  lines.push('## 失败项');
  lines.push('');
  const fails = rows.filter((r) => r.status !== 'pass');
  if (!fails.length) lines.push('无');
  else {
    for (const f of fails) {
      lines.push(`- **${f.name}** (\`${f.path}\`): ${f.notes}`);
    }
  }
  lines.push('');
  lines.push('## 控制台错误采样（已过滤 favicon/NATS/WebSocket）');
  lines.push('');
  const filtered = runtimeErrors.filter(
    (e) => !/favicon|NATS|WebSocket|ws:\/\//i.test(e),
  );
  if (!filtered.length) lines.push('无显著错误');
  else filtered.slice(0, 40).forEach((e) => lines.push(`- ${e}`));
  lines.push('');
  lines.push('## 截图目录');
  lines.push('');
  lines.push('`E:\\project\\FrameWork\\docs\\ui-verify\\`');
  lines.push('');
  fs.writeFileSync(path.join(OUT, 'report.md'), lines.join('\n'), 'utf8');
  fs.writeFileSync(path.join(OUT, 'results.json'), JSON.stringify({ rows, runtimeErrors }, null, 2), 'utf8');
}

