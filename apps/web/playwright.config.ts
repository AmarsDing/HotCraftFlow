// Playwright 端到端测试配置（雷达数据整合平台 Phase 3，T37）。
// 运行环境：本机 Docker 全栈（framework-* 容器，网关 127.0.0.1:8000）+ quasar dev（9001，代理 /api → 网关）。
// 用法：`pnpm test:e2e`（webServer 自动拉起 quasar dev；已启动则复用）。
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: 'tests/e2e',
  timeout: 90_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1, // 共享同一后端会话态（收藏/选择回写），串行防串扰
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:9001',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    viewport: { width: 1600, height: 900 },
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'pnpm dev',
    url: 'http://127.0.0.1:9001',
    reuseExistingServer: true,
    timeout: 180_000,
  },
});
