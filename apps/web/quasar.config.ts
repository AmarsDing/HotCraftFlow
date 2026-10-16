import { configure } from 'quasar/wrappers';
// 双环境 dev 代理自动识别（家=102/公司=44，按本机网段；TWIN_API_TARGET/TWIN_MAP_TARGET 可显式覆盖）
import { describeDevEnv, resolveApiTarget, resolveMapTarget } from '../scripts/dev-env.mjs';

// eslint-disable-next-line no-console
console.log(`[dev-env] ${describeDevEnv()}`);

export default configure(() => {
  return {
    supportTS: true,
    boot: ['pinia', 'theme', 'i18n', 'components', 'table-tooltip', 'table-wheel'],
    css: ['style.sass'],
    extras: ['material-icons'],
    build: {
      target: {
        browser: ['es2022', 'firefox115', 'chrome115', 'safari14'],
      },
      vueRouterMode: 'history',
      distDir: process.env.QUASAR_DIST_DIR || 'dist',
      rollupOptions: {
        external: [],
      },
      extendViteConf(viteConf) {
        viteConf.resolve = viteConf.resolve || {};
        viteConf.resolve.alias = viteConf.resolve.alias || {};
        // 大依赖独立 vendor chunk（src 实际使用已核实：three/cesium/echarts/@antv）：
        // 体积大且按页面懒加载，独立分包可长缓存，业务代码变更不再导致整包重下
        viteConf.build = viteConf.build || {};
        viteConf.build.rollupOptions = viteConf.build.rollupOptions || {};
        const prevOutput = viteConf.build.rollupOptions.output;
        const baseOutput = prevOutput && !Array.isArray(prevOutput) ? prevOutput : {};
        viteConf.build.rollupOptions.output = {
          ...baseOutput,
          manualChunks(id: string) {
            if (!id.includes('node_modules')) return undefined;
            if (id.includes('@cesium') || id.includes('/cesium/')) return 'vendor-cesium';
            if (id.includes('/three/')) return 'vendor-three';
            if (id.includes('/echarts/') || id.includes('/zrender/')) return 'vendor-echarts';
            if (id.includes('/@antv/')) return 'vendor-x6';
            return undefined;
          },
        };
      },
    },
    devServer: {
      // 数字孪生后台管理前端开发端口
      port: 9001,
      open: false,
      proxy: {
        // 单一入口：全部 API 流量（含 visualization WebSocket）经网关 :8000，
        // 由网关按 /api/v1/monitor/<svc> 前缀分流到各上游服务（5.2 流量收口）。
        // 双环境自动识别（scripts/dev-env.mjs）：公司网段→44，家庭网段→102；
        // 显式覆盖：TWIN_API_TARGET=http://127.0.0.1:8000 可切本机 Docker。
        '/api': {
          target: resolveApiTarget(),
          changeOrigin: true,
          ws: true,
        },
        // 离线底图：Martin MVT 瓦片服务（PostGIS basemap 函数），容器映射 3100→3000。
        // Martin base_path 只影响 TileJSON 不改路由（martin#1415），故代理剥掉 /martin 前缀。
        // 目标随 dev-env 环境联动（44 已部署，102 的 mapdb+martin 尚未部署，2026-08-29 实证），
        // 可用 TWIN_MAP_TARGET 显式指向任意实例。
        '/martin': {
          target: resolveMapTarget(),
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/martin/, ''),
        },
        // AWOS 配置/采集控制面（2026-09 审计修复 F2）：前端直连后端真实路径
        // （/awos/admin/*、/awos/collector/*），与 /api 同走网关分流——网关种子路由
        // 透传至 awos-admin:8219 / awos-collector:8210。原 /awos-admin、/awos-collector
        // 前端专用前缀（rewrite 剥前缀）已废弃：生产无对应反代规则时会 404。
        '/awos': {
          target: resolveApiTarget(),
          changeOrigin: true,
        },
      },
    },
    framework: {
      config: { dark: false },
      plugins: ['Dialog', 'Notify'],
    },
    animations: [],
  };
});
