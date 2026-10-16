import { defineStore } from 'pinia';
import { myNoticeService, type MyNoticeItem } from '../services';

// 我的站内信通知（顶栏铃铛 + 通知中心页共用）。
// unreadCount 由轮询驱动（30s）；铃铛面板与通知中心页各自拉取列表。
export const useMyNoticeStore = defineStore('my-notice', {
  state: () => ({
    unreadCount: 0,
    recentItems: [] as MyNoticeItem[],
    recentLoading: false,
    _timer: null as ReturnType<typeof setInterval> | null,
  }),
  actions: {
    async fetchUnreadCount() {
      try {
        const resp = await myNoticeService.countMyUnread();
        this.unreadCount = resp.count ?? 0;
      } catch {
        // 轮询场景静默失败，保留旧值
      }
    },
    async fetchRecent(count = 10) {
      this.recentLoading = true;
      try {
        const resp = await myNoticeService.listMy({ page: 1, pageSize: count });
        this.recentItems = resp.items ?? [];
        this.unreadCount = resp.unreadCount ?? 0;
      } finally {
        this.recentLoading = false;
      }
    },
    // ids 为空表示全部已读
    async markRead(ids?: number[]) {
      await myNoticeService.markMyRead(ids);
      const idSet = ids && ids.length > 0 ? new Set(ids) : null;
      this.recentItems = this.recentItems.map((it) =>
        idSet === null || (it.recipientId !== undefined && idSet.has(it.recipientId)) ? { ...it, status: 'READ' } : it,
      );
      await this.fetchUnreadCount();
    },
    startPolling(intervalMs = 30000) {
      this.stopPolling();
      void this.fetchUnreadCount();
      this._timer = setInterval(() => {
        void this.fetchUnreadCount();
      }, intervalMs);
    },
    stopPolling() {
      if (this._timer !== null) {
        clearInterval(this._timer);
        this._timer = null;
      }
    },
    reset() {
      this.stopPolling();
      this.unreadCount = 0;
      this.recentItems = [];
    },
  },
});
