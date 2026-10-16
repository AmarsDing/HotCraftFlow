import { useI18n } from 'vue-i18n';
import { Dialog } from 'quasar';

// useCascadeConfirm 物理链（机场→跑道→跑道端→绑定）级联删除确认。
// 删除确认前并发查子级存活计数（各 list 接口 pageSize:1 读 total），把「级联删除 N 条」
// 写进确认文案，避免用户删除后才发现关联数据被一并清掉。
// 计数查询失败不阻塞删除：该子级显示 ? 并沿用级联文案（与后端实际级联行为保持一致）。

/** 子级计数器；返回 undefined 表示查询失败（文案显示 ?）。 */
export type CascadeCounter = () => Promise<number | undefined>;

/**
 * 弹级联删除确认框；resolve(true)=用户确认，resolve(false)=取消。
 * @param subject 被删对象描述（如「机场 ZBAA 北京首都」），嵌入文案
 * @param baseKey 无子级时使用的 i18n 基础文案 key
 * @param withCountsKey 有子级时使用的 i18n 级联文案 key，占位符 {subject}/{detail} 由本函数填充
 * @param counters 子级计数器集合，key 为 i18n 键后缀（business.cascadeChild{Key}）
 */
export function useCascadeConfirm() {
  const { t } = useI18n();

  async function confirmCascadeDelete(
    subject: string,
    baseKey: string,
    withCountsKey: string,
    counters: Record<string, CascadeCounter>,
  ): Promise<boolean> {
    const keys = Object.keys(counters);
    const settled = await Promise.allSettled(keys.map((k) => counters[k]()));

    const children = keys.map((key, i) => {
      const r = settled[i];
      return { key, count: r.status === 'fulfilled' ? r.value : undefined };
    });

    // 全部子级确认为 0 时退化为基础文案（不啰嗦）；有失败或 >0 时展示明细
    const hasChildren = children.some((c) => c.count == null || c.count > 0);
    let message: string;
    if (!hasChildren) {
      message = t(baseKey, { subject });
    } else {
      const detail = children
        .map((c) => `${t(`business.cascadeChild${c.key}`)} ${c.count ?? '?'}`)
        .join(t('business.cascadeCountSep'));
      message = t(withCountsKey, { subject, detail });
    }

    return await new Promise<boolean>((resolve) => {
      let settledDialog = false;
      Dialog.create({
        title: t('common.delete'),
        message,
        cancel: { label: t('common.cancel'), flat: true },
        ok: { label: t('common.delete'), color: 'negative' },
        persistent: true,
      })
        .onOk(() => {
          settledDialog = true;
          resolve(true);
        })
        .onCancel(() => {
          settledDialog = true;
          resolve(false);
        })
        .onDismiss(() => {
          if (!settledDialog) resolve(false);
        });
    });
  }

  return { confirmCascadeDelete };
}
