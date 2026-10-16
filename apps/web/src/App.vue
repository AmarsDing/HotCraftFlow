<template>
  <!-- Layout 级 keep-alive（P0）：跨 Layout 往返（3D 拓扑 MainLayout ↔ 3D 详情 BlankLayout）保留 Layout 子树，
       使内层页面 keep-alive 在往返链路上真正生效；max=2 恰好容纳两个 Layout 单例 -->
  <router-view v-slot="{ Component }">
    <keep-alive :include="['MainLayout', 'BlankLayout']" :max="2">
      <component :is="Component" />
    </keep-alive>
  </router-view>
</template>
