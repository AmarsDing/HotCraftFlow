<template>
  <!-- 一键克隆：HTTPS/SSH 命令复制 · ZIP 下载 · 服务端克隆（可选） -->
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog" data-cy="clone-dialog">
      <q-card-section class="app-glass-dialog__head">
        <div class="app-glass-dialog__title">{{ t('githubRadar.cloneTitleName', { name: fullName }) }}</div>
        <div class="app-glass-dialog__subtitle">{{ t('githubRadar.cloneSubtitle') }}</div>
      </q-card-section>

      <q-card-section class="app-glass-dialog__body app-dialog-body">
        <div v-if="loading" class="row items-center justify-center q-pa-md">
          <q-spinner size="24px" color="primary" />
        </div>

        <template v-else-if="info">
          <div class="clone-dialog__row">
            <div class="clone-dialog__label">HTTPS</div>
            <code class="clone-dialog__cmd">{{ info.command }}</code>
            <q-btn
              flat
              dense
              round
              size="sm"
              icon="content_copy"
              :aria-label="t('githubRadar.copyHttps')"
              @click="copy(info.command || '')"
            >
              <q-tooltip>{{ t('githubRadar.copy') }}</q-tooltip>
            </q-btn>
          </div>

          <div class="clone-dialog__row">
            <div class="clone-dialog__label">SSH</div>
            <code class="clone-dialog__cmd">{{ info.ssh_command }}</code>
            <q-btn
              flat
              dense
              round
              size="sm"
              icon="content_copy"
              :aria-label="t('githubRadar.copySsh')"
              @click="copy(info.ssh_command || '')"
            >
              <q-tooltip>{{ t('githubRadar.copy') }}</q-tooltip>
            </q-btn>
          </div>

          <div class="clone-dialog__row">
            <div class="clone-dialog__label">ZIP</div>
            <q-btn
              outline
              dense
              no-caps
              color="primary"
              icon="archive"
              :label="t('githubRadar.downloadZip')"
              :href="info.zip_url"
              target="_blank"
              tag="a"
            />
          </div>

          <q-separator class="q-my-md" />

          <div v-if="cloneEnabled" class="clone-dialog__row clone-dialog__row--server">
            <div class="clone-dialog__label">{{ t('githubRadar.cloneServer') }}</div>
            <q-btn
              outline
              dense
              no-caps
              color="primary"
              icon="cloud_download"
              :loading="cloning"
              :label="t('githubRadar.cloneToServer')"
              @click="cloneToServer"
            />
          </div>
          <div v-else class="text-caption" style="color: var(--color-text-secondary)">
            {{ t('githubRadar.cloneServerDisabled') }}
          </div>

          <q-banner
            v-if="serverMsg"
            dense
            rounded
            class="q-mt-md"
            :class="serverOk ? 'bg-positive text-white' : 'bg-warning text-white'"
          >
            {{ serverMsg }}<span v-if="info.local_path"> → {{ info.local_path }}</span>
          </q-banner>
        </template>

        <div v-else class="text-body2" style="color: var(--color-text-secondary)">
          {{ t('githubRadar.cloneInfoFailed') }}
        </div>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn v-close-popup flat :label="t('common.close')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { radarRepoService } from '../../../services';

const props = defineProps<{
  modelValue: boolean;
  fullName: string;
  cloneEnabled?: boolean;
}>();
defineEmits<{ 'update:modelValue': [boolean] }>();

const $q = useQuasar();
const { t } = useI18n();

const loading = ref(false);
const info = ref<{
  command?: string;
  ssh_command?: string;
  zip_url?: string;
  local_path?: string;
  cloned?: boolean;
  message?: string;
} | null>(null);
const cloning = ref(false);
const serverMsg = ref('');
const serverOk = ref(false);

watch(
  () => props.modelValue,
  async (open) => {
    if (!open || !props.fullName) return;
    loading.value = true;
    serverMsg.value = '';
    serverOk.value = false;
    try {
      info.value = await radarRepoService.clone(props.fullName, false);
    } catch {
      info.value = null;
    } finally {
      loading.value = false;
    }
  },
);

async function copy(text: string) {
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    $q.notify({ type: 'positive', message: t('githubRadar.copied'), position: 'top' });
  } catch {
    $q.notify({ type: 'negative', message: t('githubRadar.copyFailed') });
  }
}

async function cloneToServer() {
  if (cloning.value) return;
  cloning.value = true;
  try {
    const res = await radarRepoService.clone(props.fullName, true);
    const raw = res as unknown as Record<string, unknown>;
    serverOk.value = !!res.cloned;
    serverMsg.value = res.message || (res.cloned ? t('githubRadar.cloneDone') : t('githubRadar.cloneFailed'));
    if (raw.local_path) info.value = { ...info.value, local_path: String(raw.local_path) };
  } catch (e) {
    serverOk.value = false;
    serverMsg.value = (e as Error).message;
  } finally {
    cloning.value = false;
  }
}
</script>

<style scoped lang="sass">
.clone-dialog__row
  display: flex
  align-items: center
  gap: var(--space-2)
  margin-bottom: var(--space-2)

.clone-dialog__row--server
  align-items: center

.clone-dialog__label
  flex: 0 0 56px
  font-size: var(--text-xs)
  font-weight: 600
  color: var(--color-text-secondary)

.clone-dialog__cmd
  flex: 1
  min-width: 0
  overflow-x: auto
  white-space: nowrap
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  border-radius: 6px
  padding: var(--space-1) var(--space-2)
  font-size: var(--text-xs)
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace
</style>
