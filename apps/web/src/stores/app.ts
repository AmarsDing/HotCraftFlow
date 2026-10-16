import { defineStore } from 'pinia';

export const useAppStore = defineStore('app', {
  state: () => ({
    loading: false,
    sidebarMini: false,
  }),
  actions: {
    toggleSidebar() {
      this.sidebarMini = !this.sidebarMini;
    },
  },
});
