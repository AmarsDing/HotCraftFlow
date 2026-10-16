<template>
  <q-page class="not-found-page column items-center justify-center q-pa-lg">
    <div class="text-h2 text-weight-bold text-grey-7 q-mb-md">404</div>
    <div class="text-subtitle1 text-grey q-mb-lg">{{ t('errors.pageNotFound') }}</div>
    <q-btn color="primary" unelevated :label="t('errors.backHome')" @click="goHome" />
  </q-page>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';

const router = useRouter();
const { t } = useI18n();

function goHome() {
  void router.push('/dashboard');
}
</script>
