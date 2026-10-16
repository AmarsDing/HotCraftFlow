import { defineStore } from 'pinia';
import { systemConfigService, type SystemConfigEntity } from '../services';

// 系统名称（system_configs.system_name，系统管理-系统配置维护）。
// 单一事实源：顶栏标题、侧栏品牌区、登录页、浏览器页签标题全部由此渲染，
// 管理员改配置后全站即时生效，无需改代码发版。
// 拉取失败降级为 i18n 静态文案，永不阻塞页面渲染。
// localStorage 缓存：登录页等未登录场景（无权限读 system_configs）沿用最近一次名称。
export const SYSTEM_NAME_CACHE_KEY = 'framework_system_name';

export function cachedSystemName(): string {
  try {
    return (localStorage.getItem(SYSTEM_NAME_CACHE_KEY) ?? '').trim();
  } catch {
    return '';
  }
}

export const useSystemNameStore = defineStore('system-name', {
  state: () => ({
    // 初始即取缓存：登录后首帧即与登录页口径一致，配置拉取回来后再校正
    name: cachedSystemName(),
    loaded: false,
  }),
  actions: {
    // 幂等拉取：仅在未加载过时请求；失败静默降级（name 保持缓存/空，display 走兜底链）
    async fetchSystemName() {
      if (this.loaded) return;
      try {
        const resp = await systemConfigService.list({
          noPaging: true,
          formValues: { configKey: 'system_name' },
        });
        const items = (resp.items ?? []) as SystemConfigEntity[];
        const hit = items.find((it) => it.configKey === 'system_name');
        const v = (hit?.configValue ?? '').trim();
        if (v) {
          this.name = v;
          // 同步缓存：登录页（未登录，无权限读配置）沿用最新名称
          try {
            localStorage.setItem(SYSTEM_NAME_CACHE_KEY, v);
          } catch {
            // 写失败仅影响未登录页沿用，忽略
          }
        }
      } catch {
        // 拦截器已提示；保持现状（缓存值或空 → i18n 兜底）
      } finally {
        this.loaded = true;
      }
    },
    // 系统配置页保存后调用：强制下次重新拉取
    invalidate() {
      this.loaded = false;
    },
    // 直接写入（系统配置页保存成功后回显用，避免再发一次请求）
    setName(v: string) {
      const s = (v ?? '').trim();
      if (s) {
        this.name = s;
        try {
          localStorage.setItem(SYSTEM_NAME_CACHE_KEY, s);
        } catch {
          // 隐私模式等写失败场景忽略，仅影响未登录页的名称沿用
        }
      }
      this.loaded = true;
    },
  },
});
