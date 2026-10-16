import { Quasar } from 'quasar';
import quasarLangEn from 'quasar/lang/en-US';
import quasarLangZh from 'quasar/lang/zh-CN';

// 覆盖官方 zh-CN 语言包中生硬的译法：
// - 分页 "每页的行数" → "每页行数"
// - 表格空态 "没有可用数据" → "暂无数据"（与各业务页引导式空态文案一致）
quasarLangZh.table.recordsPerPage = '每页行数';
quasarLangZh.table.noData = '暂无数据';

export function setQuasarLangFor(code: string) {
  if (code === 'en-US') {
    Quasar.lang.set(quasarLangEn);
  } else {
    Quasar.lang.set(quasarLangZh);
  }
}
