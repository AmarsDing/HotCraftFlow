// Vitest 配置（雷达数据整合平台 Phase 3，T36 前端集成测试）。
// 测试落位 test/vitest/__tests__/；jsdom 环境；Quasar 组件经 test/vitest/mount.ts 挂载。
import vue from '@vitejs/plugin-vue';
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      src: fileURLToPath(new URL('./src', import.meta.url)),
      // quasar package exports "node" 条件优先于 "import"，vitest（node 环境）会解析到
      // quasar.server.prod.js（SSR 构建，install 即抛 "Cannot convert undefined or null to object"）。
      // 测试在 jsdom 跑客户端组件，强制解析到客户端 ESM 构建。
      // quasar/wrappers 仅 Quasar CLI 构建注入（node 解析无此子路径）；src/stores 桶依赖它，测试走 shim。
      // 注意必须先于 quasar 键：rollup alias 字符串按 find+'/' 前缀匹配，顺序反了会被 quasar 客户端构建截胡。
      'quasar/wrappers': fileURLToPath(new URL('./test/vitest/quasar-wrappers-shim.ts', import.meta.url)),
      quasar: fileURLToPath(new URL('./node_modules/quasar/dist/quasar.client.js', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['test/vitest/**/*.test.ts'],
    setupFiles: ['test/vitest/setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html', 'json-summary'],
      reportsDirectory: 'test/vitest/coverage',
      include: [
        'src/stores/radar-workbench.ts',
        'src/pages/radar-workbench/**/*.{ts,vue}',
      ],
      exclude: ['src/pages/radar-workbench/components/views/**'],
      thresholds: {
        statements: 80,
        branches: 80,
        functions: 80,
        lines: 80,
      },
    },
  },
});
