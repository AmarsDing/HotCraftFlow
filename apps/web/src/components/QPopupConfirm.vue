<template>
  <q-popup-proxy :offset="[0, 4]" transition-show="scale" transition-hide="scale">
    <q-card class="confirm-popup">
      <q-card-section class="confirm-popup__message">
        <slot />
      </q-card-section>
      <q-card-actions align="right" class="confirm-popup__actions">
        <q-btn v-close-popup flat dense :label="cancelLabel" class="confirm-popup__btn" />
        <q-btn
          v-close-popup
          unelevated
          dense
          color="negative"
          :label="confirmLabel"
          class="confirm-popup__btn confirm-popup__btn--confirm"
          @click="onConfirm"
        />
      </q-card-actions>
    </q-card>
  </q-popup-proxy>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

interface Props {
  confirmLabel?: string;
  cancelLabel?: string;
}

const props = withDefaults(defineProps<Props>(), {
  confirmLabel: undefined,
  cancelLabel: undefined,
});

const emit = defineEmits<{
  confirm: [];
}>();

const { t } = useI18n();

const confirmLabel = computed(() => props.confirmLabel ?? t('common.confirm'));
const cancelLabel = computed(() => props.cancelLabel ?? t('common.cancel'));

function onConfirm() {
  emit('confirm');
}
</script>

<style lang="sass">
.confirm-popup
  min-width: 180px
  border-radius: 12px
  background: var(--glass-elevated)
  border: 1px solid var(--glass-border)
  box-shadow: var(--shadow-warm)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))

.confirm-popup__message
  padding: 12px 16px
  font-size: 13px
  line-height: 1.5
  color: var(--color-text-primary)

.confirm-popup__actions
  padding: 8px 12px 12px
  gap: var(--space-2)

.confirm-popup__btn
  min-height: 28px
  padding: 0 12px
  font-size: 12px
  font-weight: 600

.confirm-popup__btn--confirm
  border-radius: 8px

body.body--dark .confirm-popup
  background: color-mix(in srgb, var(--glass-elevated) 92%, transparent)
  border-color: var(--glass-border)
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.32)
</style>
