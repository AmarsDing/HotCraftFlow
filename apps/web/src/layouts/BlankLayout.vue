<template>
  <div class="blank-layout-root fit">
    <router-view v-slot="{ Component, route }">
      <!-- 设备 3D 详情页 + 综合运维中心 keep-alive：返回再进入免重建场景/着色器（进入卡顿修复 P0/S4）；
           fullPath 分键隔离不同设备实例，max 限制驻留的 WebGL 上下文数量；
           integrated-ops 用稳定 key（分享链接 query lyr/cur/cam/alarm/dev 不分键，保实例复用，
           query 变化重放由页面 onActivated 处理）；其它路由保持 fullPath 分键不变；
           同 Layout 的 visual-twin/airport-twin 等全屏页不在 include，行为不变；
           meta.layout 判定非本 Layout 路由渲空（防 App 层 keep-alive 停用时误挂载对方子页面） -->
      <!-- ErrorBoundary 兜底：动态路由页加载失败/渲染抛错时给错误态+重试，而非静默空白。
           reset-key 换路由清错误态（不重建子树，保住内层 keep-alive 缓存） -->
      <ErrorBoundary :reset-key="route.fullPath">
        <keep-alive :include="['Device3dDetailPage', 'IntegratedOpsPage']" :max="4">
          <component :is="Component" :key="viewKey(route)" />
        </keep-alive>
      </ErrorBoundary>
    </router-view>

    <!-- 设备 360° 全局抽屉宿主（可视化信息关联重组 P1-T2）：room3d 等 BlankLayout 全屏页可用 -->
    <Device360Host />
  </div>
</template>

<script setup lang="ts">
import type { RouteLocationNormalizedLoaded } from 'vue-router';
import Device360Host from '../components/device-360/Device360Host.vue';
import ErrorBoundary from '../components/ErrorBoundary.vue';

// App 层 keep-alive include 匹配名（跨 Layout 往返时保留 BlankLayout 子树 → 内层详情页缓存生效）
defineOptions({ name: 'BlankLayout' });

/** 缓存分键：integrated-ops 用路由名稳定 key（分享链接 query 不产生多实例）；其余按 fullPath 隔离 */
function viewKey(route: RouteLocationNormalizedLoaded): string {
  return route.name === 'integrated-ops' ? String(route.name) : route.fullPath;
}
</script>
