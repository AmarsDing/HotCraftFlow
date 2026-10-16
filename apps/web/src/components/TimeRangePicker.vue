<template>
  <!-- 不要手动 @click 打开弹层：QPopupProxy 的 useAnchor 已在父锚点自动绑定
       click→toggle（且标记 qAnchorHandled 防自身被外点关闭）。手动 show() 不打标记，
       同一次点击冒泡到 document 会被 QMenu 判为外点立即关回（timeline 实证：
       show@11ms → toggle@19ms 互相抵消）。q-input 上挂 @click 同样无效——
       Quasar2 QInput 将 click 声明为组件 emit 却从不发射（2.24.0 实证） -->
  <div class="trp-anchor">
    <q-input
      :model-value="displayText"
      :label="label || undefined"
      :placeholder="placeholder || t('common.timeRange.label')"
      :rules="rules"
      :disable="disable"
      readonly
      dense
      outlined
      class="time-range-picker"
      @keydown.enter.space.down.prevent="onTriggerKeydown"
    >
      <template #prepend>
        <q-icon name="schedule" class="trp-icon" />
      </template>
      <template #append>
        <!-- 跨度徽章：关闭态也能一眼看到当前区间量级（状态标识） -->
        <span v-if="appliedSpan" class="trp-span">{{ appliedSpan }}</span>
        <!-- 清除按钮：图标也可达（role/tabindex/aria-label + Enter/Space 键盘触发）；
             keydown 加 .stop 防冒泡到输入框的打开弹层快捷键 -->
        <q-icon
          v-if="clearable && hasValue"
          name="close"
          class="trp-clear cursor-pointer"
          role="button"
          tabindex="0"
          :aria-label="t('common.timeRange.clear')"
          @click.stop="onClear"
          @keydown.enter.space.stop.prevent="onClear"
        />
        <q-icon name="expand_more" class="trp-caret" :class="{ 'trp-caret--open': popupOpen }" />
      </template>
    </q-input>
    <!-- 弹层锚定整个输入框（trp-anchor），定位可预期 -->
    <q-popup-proxy
      ref="popupRef"
      transition-show="scale"
      transition-hide="scale"
      @show="onPopupShow"
      @hide="popupOpen = false"
    >
      <div class="trp-panel" @click.stop>
        <!-- 状态横幅：当前生效区间（解析后的真实起止 + 跨度），任何操作前先明确现状 -->
        <div class="trp-status">
          <q-icon name="verified" class="trp-status__icon" />
          <span class="trp-status__label">{{ t('common.timeRange.applied') }}</span>
          <span class="trp-status__text">{{ appliedText }}</span>
          <span v-if="appliedSpan" class="trp-status__span">{{ appliedSpan }}</span>
        </div>

        <div class="trp-body">
          <!-- 预设竖排（点选即生效并关闭）；active 行带 ✓ 状态标识 -->
          <div v-if="showPresets" class="trp-rail">
            <div class="trp-rail__title">{{ t('common.timeRange.quick') }}</div>
            <!-- 原生 button：天然获得 Tab 聚焦与 Enter/Space 触发（键盘/读屏可达），视觉由 sass 复位保持不变 -->
            <button
              v-for="p in presets"
              :key="p"
              type="button"
              class="trp-preset"
              :class="{ 'trp-preset--active': p === modelValue.preset }"
              @click.stop="onPickPreset(p)"
            >
              <q-icon :name="presetIcon(p)" class="trp-preset__icon" />
              <span class="trp-preset__name ellipsis">{{ t(`common.timeRange.${p}`) }}</span>
              <q-icon v-if="p === modelValue.preset" name="check" class="trp-preset__check" />
            </button>
          </div>

          <!-- 自定义区间：单日历 range 点选起止（自动排序无需互相禁用）；minute 精度补充时间输入 -->
          <div v-if="customVisible" class="trp-main">
            <div class="trp-main__head">
              <span class="trp-main__title">{{ t('common.timeRange.custom') }}</span>
              <!-- 草稿状态标识：改动未应用 → 琥珀；起止非法 → 红 -->
              <span v-if="invalidRange" class="trp-badge trp-badge--invalid">
                <q-icon name="error_outline" size="12px" />{{ t('common.timeRange.invalid') }}
              </span>
              <span v-else-if="draftDirty" class="trp-badge trp-badge--dirty">
                <q-icon name="edit" size="12px" />{{ t('common.timeRange.dirty') }}
              </span>
            </div>
            <q-date
              :model-value="rangeModel"
              range
              mask="YYYY-MM-DD"
              minimal
              class="trp-calendar"
              @update:model-value="onCalendarPick"
            />
            <div v-if="granularity === 'minute'" class="trp-times">
              <q-input
                v-model="dsTime"
                class="trp-time"
                mask="##:##"
                fill-mask
                dense
                outlined
                :error="!!dsTime && !timeRe.test(dsTime)"
                hide-bottom-space
                @update:model-value="onDraftEdit"
              >
                <template #prepend><q-icon name="schedule" size="16px" /></template>
              </q-input>
              <q-icon name="east" class="trp-times__sep" />
              <q-input
                v-model="deTime"
                class="trp-time"
                mask="##:##"
                fill-mask
                dense
                outlined
                :error="!!deTime && !timeRe.test(deTime)"
                hide-bottom-space
                @update:model-value="onDraftEdit"
              >
                <template #prepend><q-icon name="schedule" size="16px" /></template>
              </q-input>
            </div>
            <div class="trp-footer">
              <div class="trp-preview" :title="previewText">
                <q-icon name="timelapse" />
                <span class="trp-preview__text">{{ previewText }}</span>
                <span v-if="spanText" class="trp-preview__span">{{ spanText }}</span>
              </div>
              <div class="trp-footer__actions">
                <q-btn outline dense icon="restart_alt" :label="t('common.reset')" @click.stop="onResetDraft" />
                <!-- 应用门槛=存在可应用的草稿变更（draftDirty 已含 canApply 与"不同于生效值"
                     判定）：打开弹层未改动直接点应用，会把滑动预设静默转成 custom 冻结窗
                     （24h 滑窗变死窗、数据不再跟随，审查实证 P-B） -->
                <q-btn
                  unelevated
                  dense
                  color="primary"
                  icon="check"
                  :label="t('common.apply')"
                  :disable="!draftDirty"
                  :class="{ 'trp-apply--pulse': draftDirty }"
                  @click.stop="onApply"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </q-popup-proxy>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { QPopupProxy, ValidationRule } from 'quasar';
import { formatDuration } from '../utils/datetime';
import { ensureShiftWindows } from '../utils/shiftWindow';
import {
  DEFAULT_TIME_RANGE_PRESETS,
  formatLocalDate,
  formatLocalDateTime,
  parseLocalTime,
  resolveTimeRangeMs,
  type TimeRangeGranularity,
  type TimeRangeValue,
} from '../utils/timerange';

// 时间范围选择器（公共组件）：
//  - 面板顶部常驻「当前生效」状态横幅（解析后的真实起止 + 跨度徽章）
//  - 预设竖排点选即生效；自定义区单日历 range 点选起止（q-date 自动排序，无需互相钳制）
//  - 草稿改动未应用 → 「未应用」琥珀徽章 + 应用按钮脉冲；起止非法 → 红色错误徽章
//  - 主控件 append 区常驻跨度徽章（关闭态也可见当前区间量级）
// 样式集中在 src/css/theme/_time-range-picker.sass（玻璃拟态 + 主题强调色，昼夜自适应）
const props = withDefaults(
  defineProps<{
    modelValue: TimeRangeValue;
    /** 预设档位集合；传 [] 隐藏预设区（表单纯自定义场景），custom 项由 'custom' key 控制 */
    presets?: string[];
    /** minute=精确到分钟（YYYY-MM-DD HH:mm），date=仅日期（YYYY-MM-DD） */
    granularity?: TimeRangeGranularity;
    label?: string;
    placeholder?: string;
    rules?: ValidationRule[];
    disable?: boolean;
    /** 可清空：显示清除按钮，清空后回 custom 空值（可选表单"不限/自动"语义） */
    clearable?: boolean;
  }>(),
  {
    presets: () => [...DEFAULT_TIME_RANGE_PRESETS],
    granularity: 'minute',
    label: '',
    placeholder: '',
    rules: undefined,
    disable: false,
    clearable: false,
  },
);

const emit = defineEmits<{
  'update:modelValue': [value: TimeRangeValue];
  change: [];
}>();

const { t } = useI18n();

const popupRef = ref<QPopupProxy>();
const popupOpen = ref(false);
const customExpanded = ref(false);
// 草稿是否被本次会话改动过（驱动「未应用」徽章与按钮脉冲；预设滚动差值不误判）
const draftTouched = ref(false);

// 草稿模型：单日历 range（from/to 日期）+ minute 精度时间片段
interface DateRangeModel {
  from: string;
  to: string;
}
const rangeModel = ref<DateRangeModel | null>(null);
const dsTime = ref('');
const deTime = ref('');

const timeRe = /^([01]\d|2[0-3]):[0-5]\d$/;

const showPresets = computed(() => props.presets.length > 0);
const customVisible = computed(() => !showPresets.value || customExpanded.value);

// range 单日历：只点一个日期时 to 回退 from（单日区间合法）
const dsDate = computed(() => rangeModel.value?.from ?? '');
const deDate = computed(() => rangeModel.value?.to || rangeModel.value?.from || '');

const draftStart = computed(() => combine(dsDate.value, dsTime.value));
const draftEnd = computed(() => combine(deDate.value, deTime.value));

function combine(d: string, tm: string): string {
  if (!d) return '';
  return props.granularity === 'date' ? d : `${d} ${tm || '00:00'}`;
}

function split(full: string): { d: string; tm: string } {
  if (!full) return { d: '', tm: '' };
  const [d = '', tm = ''] = full.split(' ');
  return { d, tm };
}

/** 按精度格式化毫秒为模型值字符串 */
function fmt(ms: number): string {
  const d = new Date(ms);
  return props.granularity === 'date' ? formatLocalDate(d) : formatLocalDateTime(d);
}

// ── 生效状态（横幅 + 主控件跨度徽章共用同一解析口径） ──
const applied = computed(() => {
  const { startMs, endMs } = resolveTimeRangeMs(props.modelValue);
  return { start: fmt(startMs), end: fmt(endMs), startMs, endMs };
});

const appliedText = computed(() => `${applied.value.start} ~ ${applied.value.end}`);

function spanLabel(spanMs: number): string {
  if (props.granularity === 'date') {
    return t('common.timeRange.days', { n: Math.round(spanMs / 86_400_000) + 1 });
  }
  return formatDuration(spanMs / 1000);
}

const hasValue = computed(() => {
  const v = props.modelValue;
  return v.preset !== 'custom' || !!v.start || !!v.end;
});

const appliedSpan = computed(() => (hasValue.value ? spanLabel(applied.value.endMs - applied.value.startMs) : ''));

// ── 主控件显示文本 ──
const displayText = computed(() => {
  const v = props.modelValue;
  if (v.preset !== 'custom') {
    return props.presets.includes(v.preset) ? t(`common.timeRange.${v.preset}`) : '';
  }
  if (!v.start && !v.end) return '';
  return `${compact(v.start)} ~ ${compact(v.end)}`;
});

// 主控件宽度有限：minute 精度省略当年年份（MM-DD HH:mm）
function compact(str: string): string {
  if (!str) return '…';
  if (props.granularity === 'date') return str;
  const yr = String(new Date().getFullYear());
  return str.startsWith(`${yr}-`) ? str.slice(5) : str;
}

// ── 弹层生命周期 ──
// 键盘打开弹层：readonly 输入框无原生打开路径，Enter/Space/↓ 显式 show()。
// 与顶部注释的 click 链路不同——键盘触发无后续 document click 冒泡，不会被 QMenu 判外点关回。
function onTriggerKeydown() {
  if (props.disable) return;
  popupRef.value?.show();
}

function onPopupShow() {
  popupOpen.value = true;
  customExpanded.value = props.modelValue.preset === 'custom';
  draftTouched.value = false;
  // 班次预设（shift/prevShift）真实起止依赖排班缓存：打开弹层时保新鲜（TTL 5min 内空转）；
  // 缓存到位后 resolveTimeRangeMs 经响应式 shiftWindows 重算，横幅起止自动从 8h 降级窗切换为真实班次
  void ensureShiftWindows();
  // 草稿初始化 = 当前生效区间的解析值（预设也落成真实起止，所见即所得）
  const s = split(fmt(applied.value.startMs));
  const e = split(fmt(applied.value.endMs));
  rangeModel.value = { from: s.d, to: e.d };
  dsTime.value = s.tm || '00:00';
  deTime.value = e.tm || '00:00';
}

// ── 清空（clearable）──
function onClear() {
  emit('update:modelValue', { preset: 'custom', start: '', end: '' });
  emit('change');
}

// ── 预设 ──
function presetIcon(p: string): string {
  if (p === 'custom') return 'tune';
  if (p === 'today') return 'today';
  if (p === 'shift') return 'work_history';
  if (p === 'prevShift') return 'history';
  if (p.endsWith('d')) return 'date_range';
  if (p === 'last15m' || p === 'last1h') return 'schedule';
  return 'timelapse';
}

function onPickPreset(p: string) {
  if (p === 'custom') {
    customExpanded.value = true;
    return;
  }
  emit('update:modelValue', { preset: p, start: '', end: '' });
  emit('change');
  popupRef.value?.hide();
}

// ── 自定义草稿 ──
// q-date range 点击自动保证 from <= to（先大后小自动交换），无需旧版互相禁用逻辑
function onCalendarPick(v: DateRangeModel | null) {
  rangeModel.value = v;
  draftTouched.value = true;
}

function onDraftEdit() {
  draftTouched.value = true;
}

const timeValid = computed(
  () => props.granularity === 'date' || (timeRe.test(dsTime.value) && timeRe.test(deTime.value)),
);

const canApply = computed(() => {
  if (!dsDate.value || !deDate.value || !timeValid.value) return false;
  const s = parseLocalTime(draftStart.value);
  const e = parseLocalTime(draftEnd.value);
  if (Number.isNaN(s) || Number.isNaN(e)) return false;
  // date 精度允许同日（单日包含式区间）；minute 精度严格 开始<结束
  return props.granularity === 'date' ? s <= e : s < e;
});

// 起止已填但整体非法（如 minute 精度同日时间倒置）：红色错误状态标识
const invalidRange = computed(() => {
  if (!dsDate.value || !deDate.value || !timeValid.value) return false;
  return !canApply.value;
});

// 草稿与生效值不一致 → 「未应用」徽章 + 应用按钮脉冲
const draftDirty = computed(
  () =>
    draftTouched.value &&
    canApply.value &&
    (draftStart.value !== applied.value.start || draftEnd.value !== applied.value.end),
);

const previewText = computed(() => `${draftStart.value || '…'} ~ ${draftEnd.value || '…'}`);

const spanText = computed(() => {
  if (!canApply.value) return '';
  return spanLabel(parseLocalTime(draftEnd.value) - parseLocalTime(draftStart.value));
});

// 重置=撤销草稿改动、恢复当前生效区间（值班员对「重置」的预期是回到已应用状态；
// 旧行为"填入默认 24h/30d" 与预设栏功能冗余且语义相悖，审查实证 P-E）。
// 复位后 draftTouched=false → 未应用徽章消失、应用按钮回到禁用（无变更可应用）。
function onResetDraft() {
  const s = split(fmt(applied.value.startMs));
  const e = split(fmt(applied.value.endMs));
  rangeModel.value = { from: s.d, to: e.d };
  dsTime.value = s.tm || '00:00';
  deTime.value = e.tm || '00:00';
  draftTouched.value = false;
}

function onApply() {
  if (!canApply.value) return;
  emit('update:modelValue', { preset: 'custom', start: draftStart.value, end: draftEnd.value });
  emit('change');
  draftTouched.value = false;
  popupRef.value?.hide();
}
</script>
