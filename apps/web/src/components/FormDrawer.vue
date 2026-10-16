<template>
  <q-dialog
    v-model="open"
    position="right"
    full-height
    transition-show="slide-left"
    transition-hide="slide-right"
    :class="['form-drawer-dialog', 'alarm-nested-dialog', { 'is-dense': dense }]"
  >
    <q-card
      class="form-drawer-card column no-wrap"
      :style="{ width: `${width}px`, '--form-drawer-width': `${width}px` }"
    >
      <q-card-section
        class="form-drawer-header row items-center justify-between"
        :class="dense ? 'q-px-md q-py-sm' : 'q-px-lg q-py-md'"
      >
        <div class="form-drawer-title">{{ title }}</div>
        <q-btn
          class="form-drawer-close"
          flat
          round
          dense
          icon="close"
          :aria-label="t('common.close')"
          @click="open = false"
        />
      </q-card-section>
      <q-separator class="form-drawer-separator" />
      <q-scroll-area class="col">
        <q-form
          ref="formRef"
          greedy
          :class="dense ? 'q-px-md q-py-sm' : 'q-px-lg q-py-lg q-gutter-md'"
          @submit.prevent="onSubmit"
        >
          <slot />
        </q-form>
      </q-scroll-area>
      <q-separator class="form-drawer-separator" />
      <q-card-section
        v-if="showSubmit"
        class="form-drawer-footer row items-center justify-end q-gutter-sm"
        :class="dense ? 'q-px-md q-py-sm' : 'q-px-lg q-py-md'"
      >
        <q-btn flat :label="t('common.cancel')" :disable="saving" @click="open = false" />
        <!-- 额外提交动作（如"保存并继续"），由使用方决定显隐与行为 -->
        <slot name="footer-extra" />
        <q-btn unelevated color="primary" :label="t('common.save')" :loading="saving" @click="onSubmit" />
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { QForm } from 'quasar';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    title: string;
    saving?: boolean;
    // 只读抽屉（如日志查看）置 false 隐藏底部取消/保存按钮
    showSubmit?: boolean;
    // 抽屉宽度（px），默认 480；宽表/宽表单场景按需加宽
    width?: number;
    // 密排：收紧页边距，且不再给无 bottom 的字段补 20px（字段多的配置抽屉用）
    dense?: boolean;
  }>(),
  { saving: false, showSubmit: true, width: 480, dense: false },
);

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  submit: [];
  // q-form 校验未通过时触发（submit 被拦截），使用方可据此展示非 q-field 控件的必填错误（如文件选择）
  'submit-invalid': [];
}>();

const formRef = ref<QForm | null>(null);

const open = ref(props.modelValue);
watch(
  () => props.modelValue,
  (v) => {
    open.value = v;
  },
);
watch(open, (v) => emit('update:modelValue', v));

async function onSubmit() {
  const valid = await formRef.value?.validate();
  if (valid) emit('submit');
  else emit('submit-invalid');
}

// 暴露校验能力：使用方自定义提交按钮（footer-extra 插槽）需先触发同一套 q-form 校验
async function validate(): Promise<boolean> {
  return (await formRef.value?.validate()) ?? false;
}
defineExpose({ validate });
</script>

<style lang="sass">
.form-drawer-dialog
  .q-dialog__inner--right
    padding: 0

  .form-drawer-card
    width: var(--form-drawer-width, 480px)
    max-width: 90vw
    height: 100vh
    border-radius: 20px 0 0 20px
    background: color-mix(in srgb, var(--glass-elevated) 50%, var(--color-surface-solid))
    border-left: 1px solid var(--glass-border)
    box-shadow: -12px 0 40px rgba(0, 0, 0, 0.22)
    backdrop-filter: blur(var(--glass-blur-elevated))
    -webkit-backdrop-filter: blur(var(--glass-blur-elevated))

  .form-drawer-header
    flex-shrink: 0

  .form-drawer-title
    font-size: 18px
    font-weight: 700
    color: var(--color-text-heading)
    letter-spacing: 0.2px

  .form-drawer-close
    color: var(--color-text-secondary)
    transition: color 0.2s, background 0.2s

    &:hover
      color: var(--color-text-primary)
      background: var(--glass-surface-hover)

  .form-drawer-separator
    background: var(--glass-border)
    flex-shrink: 0

  .form-drawer-footer
    flex-shrink: 0

  // 表单字段盒高对齐（2026-08 报表模块审计确立）：Quasar 仅对有 rules/hint 的字段渲染
  // .q-field__bottom（20px 错误/提示预留位），无 bottom 的字段盒高 34px vs 有 bottom 54px，
  // 同一张表单里垂直间距不均。统一给无 bottom 的字段补 20px，使所有字段盒高一致（54px），
  // 控件本体等高（34px dense）不变，错误/hint 出现时布局不再跳动。
  &:not(.is-dense)
    .q-form .q-field:not(.q-field--with-bottom)
      padding-bottom: 20px

  &.is-dense
    .form-drawer-title
      font-size: 16px

body.body--dark
  .form-drawer-dialog .form-drawer-card
    background: color-mix(in srgb, var(--glass-elevated) 45%, var(--color-surface-solid))
    box-shadow: -12px 0 40px rgba(0, 0, 0, 0.45), 0 0 24px color-mix(in srgb, var(--color-accent) 10%, transparent)
    border-color: color-mix(in srgb, var(--glass-border) 80%, transparent)
</style>
