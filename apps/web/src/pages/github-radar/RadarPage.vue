<template>
  <div class="radar-page">
    <!-- 筛选栏：FrameWork .app-filter-bar -->
    <div class="app-filter-bar q-mb-md">
      <q-btn-toggle
        v-model="mode"
        class="app-filter-bar__field--auto"
        dense
        unelevated
        toggle-color="primary"
        :options="[
          { label: '热门', value: 'trending' },
          { label: '新星', value: 'rising' },
        ]"
      />
      <q-btn-toggle
        v-if="mode === 'trending'"
        v-model="since"
        class="app-filter-bar__field--sm"
        dense
        outline
        toggle-color="primary"
        :options="[
          { label: '今日', value: 'today' },
          { label: '本周', value: 'week' },
        ]"
      />
      <q-input
        v-model="query"
        class="app-filter-bar__field"
        dense
        outlined
        clearable
        :placeholder="mode === 'rising' ? '关键词发现新星' : '搜索仓库…'"
        debounce="400"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="language"
        class="app-filter-bar__field--sm"
        dense
        outlined
        clearable
        emit-value
        map-options
        :options="languageOptions"
        placeholder="语言"
      />
      <q-input
        v-if="mode === 'trending'"
        v-model="topic"
        class="app-filter-bar__field--sm"
        dense
        outlined
        clearable
        placeholder="Topic"
        debounce="400"
      />
      <q-toggle
        v-if="mode === 'trending'"
        v-model="onlyNew"
        dense
        label="仅新晋"
        class="app-filter-bar__field--auto"
      />
      <div class="app-filter-bar__actions">
        <q-btn outline dense no-caps label="重置" @click="clearFilters" />
        <q-btn flat dense round icon="refresh" aria-label="刷新" @click="reload">
          <q-tooltip>刷新</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          icon="notifications"
          aria-label="通知"
          @click="notifyOpen = true"
        >
          <q-badge v-if="unreadNotify > 0" color="negative" floating>{{ unreadNotify }}</q-badge>
          <q-tooltip>通知</q-tooltip>
        </q-btn>
        <q-btn flat dense round icon="settings" aria-label="设置" @click="settingsOpen = true">
          <q-tooltip>设置</q-tooltip>
        </q-btn>
        <q-btn
          v-if="!isWide"
          flat
          dense
          no-caps
          color="primary"
          :label="`盯盘 (${watches.length})`"
          icon="bookmarks"
          @click="railOpen = true"
        />
      </div>
    </div>

    <q-banner
      v-if="stale"
      dense
      rounded
      class="q-mb-md"
      style="background: var(--color-warning-soft, color-mix(in srgb, var(--color-warning) 16%, transparent))"
    >
      数据可能过期{{ staleReason ? `：${staleReason}` : '' }}
    </q-banner>

    <div class="radar-page__body row no-wrap">
      <div class="radar-page__main col">
        <div class="row items-center q-mb-sm">
          <div class="text-caption" style="color: var(--color-text-secondary)">
            <template v-if="refreshedLabel">刷新于 {{ refreshedLabel }}</template>
            <template v-else>尚未刷新</template>
          </div>
          <q-space />
          <span
            v-if="mode === 'rising'"
            class="text-caption"
            style="color: var(--color-text-secondary)"
          >新星 · 按增速</span>
        </div>

        <div v-if="loading && !items.length" class="radar-grid">
          <div v-for="n in 6" :key="n" class="dash-card" style="min-height: 160px; opacity: 0.5" />
        </div>
        <div v-else-if="items.length" class="radar-grid">
          <RepoCard
            v-for="repo in items"
            :key="repo.id"
            :repo="repo"
            :watched="watchedNames.has(repo.full_name)"
            :since="since"
            @open="onOpen"
            @watch="onWatch"
          />
        </div>
        <div v-else class="dash-card dash-card__empty q-pa-xl text-center">
          <q-icon name="travel_explore" size="48px" color="grey" />
          <div class="text-h6 q-mt-md">{{ emptyTitle }}</div>
          <div class="text-caption q-mt-sm" style="color: var(--color-text-secondary)">{{ emptyHint }}</div>
          <q-btn
            v-if="hasFilters"
            class="q-mt-md"
            outline
            color="primary"
            label="清除过滤"
            @click="clearFilters"
          />
        </div>
      </div>

      <div v-if="isWide" class="radar-page__rail">
        <WatchRail
          :items="watches"
          :unread="unreadEvents"
          :events="events"
          @add="onAddWatch"
          @remove="onRemoveWatch"
        />
      </div>
    </div>

    <q-drawer v-if="!isWide" v-model="railOpen" side="right" overlay bordered :width="320">
      <WatchRail
        :items="watches"
        :unread="unreadEvents"
        :events="events"
        @add="onAddWatch"
        @remove="onRemoveWatch"
      />
    </q-drawer>

    <!-- 详情 -->
    <q-dialog v-model="detailOpen">
      <q-card class="app-dialog-card app-dialog-card--md" style="min-width: 360px">
        <q-card-section>
          <div class="text-h6">{{ detailRepo?.full_name }}</div>
          <div class="text-body2 q-mt-sm" style="color: var(--color-text-secondary)">
            {{ detailRepo?.description || '暂无描述' }}
          </div>
          <div class="q-mt-md text-caption">
            ★ {{ detailRepo?.stars }} · Fork {{ detailRepo?.forks }} · +{{ detailRepo?.stars_delta }}
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="关闭" v-close-popup />
          <q-btn
            unelevated
            color="primary"
            :label="detailRepo && watchedNames.has(detailRepo.full_name) ? '已盯盘' : '加入盯盘'"
            :disable="!detailRepo || watchedNames.has(detailRepo.full_name)"
            @click="detailRepo && onWatch(detailRepo)"
          />
          <q-btn
            outline
            color="primary"
            label="GitHub"
            :href="detailRepo?.html_url"
            target="_blank"
            tag="a"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 通知 -->
    <q-dialog v-model="notifyOpen">
      <q-card class="app-dialog-card app-dialog-card--sm" style="min-width: 320px">
        <q-card-section class="text-h6">通知</q-card-section>
        <q-card-section>
          <div v-for="n in notifications" :key="n.id" class="q-py-sm">
            <div class="text-body2 text-weight-medium">{{ n.title }}</div>
            <div class="text-caption" style="color: var(--color-text-secondary)">{{ n.body }}</div>
          </div>
          <div v-if="!notifications.length" class="text-caption">暂无通知</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="关闭" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 设置 -->
    <q-dialog v-model="settingsOpen">
      <q-card class="app-dialog-card app-dialog-card--sm" style="min-width: 360px">
        <q-card-section class="text-h6">雷达设置</q-card-section>
        <q-card-section>
          <div class="text-caption q-mb-sm" style="color: var(--color-text-secondary)">
            GitHub Token（仅存服务端，脱敏回显）
            <template v-if="configMasked"> · 当前 {{ configMasked }}</template>
          </div>
          <q-input v-model="tokenInput" dense outlined type="password" placeholder="ghp_…" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="取消" v-close-popup />
          <q-btn unelevated color="primary" label="保存" :loading="savingConfig" @click="saveConfig" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import {
  radarTrendingService,
  radarWatchService,
  radarNotifyService,
  radarConfigService,
} from 'src/services'
/**
 * github热门追踪 — FrameWork 壳内页面
 * UI：强制使用平台 .dash-card / .app-filter-bar / CSS 变量，禁止旧 radar-* 独立色板。
 *
 * API：暂时 src/services radar*Service (kratos gen) + 本地类型。
 * 待 buf generate 后改为 services/kratos 生成客户端，禁止手改 gen。
 */
import { ref, computed, watch, onMounted } from 'vue'
import { useQuasar, Notify } from 'quasar'
import RepoCard from './components/RepoCard.vue'
import WatchRail from './components/WatchRail.vue'
import type { RepoItem, WatchItem, WatchEvent, RadarNotification } from './types'

const $q = useQuasar()
function asRepo(r: Record<string, unknown>): RepoItem {
  return {
    id: String(r.id ?? ''),
    full_name: String(r.full_name ?? r.fullName ?? ''),
    description: String(r.description ?? ''),
    language: String(r.language ?? ''),
    topics: (r.topics as string[]) || [],
    stars: Number(r.stars ?? 0),
    forks: Number(r.forks ?? 0),
    rank: Number(r.rank ?? 0),
    stars_delta: Number(r.stars_delta ?? r.starsDelta ?? 0),
    is_new: Boolean(r.is_new ?? r.isNew),
    html_url: String(r.html_url ?? r.htmlUrl ?? ''),
    sparkline: ((r.sparkline as number[]) || []).map(Number),
  }
}

function asWatch(r: Record<string, unknown>): WatchItem {
  return {
    id: String(r.id ?? ''),
    full_name: String(r.full_name ?? r.fullName ?? ''),
    stars: Number(r.stars ?? 0),
    stars_delta: Number(r.stars_delta ?? r.starsDelta ?? 0),
    unread_count: Number(r.unread_count ?? r.unreadCount ?? 0),
    sparkline: ((r.sparkline as number[]) || []).map(Number),
    html_url: String(r.html_url ?? r.htmlUrl ?? ''),
  }
}

function asEvent(r: Record<string, unknown>): WatchEvent {
  return {
    id: String(r.id ?? ''),
    full_name: String(r.full_name ?? r.fullName ?? ''),
    kind: String(r.kind ?? ''),
    message: String(r.message ?? ''),
    created_at: String(r.created_at ?? r.createdAt ?? ''),
    read: Boolean(r.read),
  }
}

function asNotify(r: Record<string, unknown>): RadarNotification {
  return {
    id: String(r.id ?? ''),
    title: String(r.title ?? ''),
    body: String(r.body ?? ''),
    kind: String(r.kind ?? ''),
    created_at: String(r.created_at ?? r.createdAt ?? ''),
    read: Boolean(r.read),
  }
}

async function loadBoard() {
  loading.value = true
  try {
    if (mode.value === 'rising') {
      const data = await radarTrendingService.searchRising({
        query: query.value || undefined,
        language: language.value || undefined,
      })
      const raw = data as unknown as Record<string, unknown>
      items.value = ((raw.items as unknown[]) || []).map((x) => asRepo(x as Record<string, unknown>))
      stale.value = Boolean(raw.stale)
      staleReason.value = String(raw.stale_reason ?? raw.staleReason ?? '')
      refreshedAt.value = String(raw.refreshed_at ?? raw.refreshedAt ?? '')
    } else {
      const data = await radarTrendingService.listTrending({
        since: since.value,
        language: language.value || undefined,
        topic: topic.value || undefined,
        onlyNew: onlyNew.value || undefined,
      })
      const raw = data as unknown as Record<string, unknown>
      items.value = ((raw.items as unknown[]) || []).map((x) => asRepo(x as Record<string, unknown>))
      stale.value = Boolean(raw.stale)
      staleReason.value = String(raw.stale_reason ?? raw.staleReason ?? '')
      refreshedAt.value = String(raw.refreshed_at ?? raw.refreshedAt ?? '')
      languageTop.value = (raw.language_top ?? raw.languageTop ?? {}) as Record<string, number>
    }
  } catch (e) {
    Notify.create({ type: 'negative', message: `加载失败：${(e as Error).message}` })
  } finally {
    loading.value = false
  }
}

async function loadSide() {
  try {
    const [w, ev, n, cfg] = await Promise.all([
      radarWatchService.list(),
      radarWatchService.listEvents(),
      radarNotifyService.list(),
      radarConfigService.get(),
    ])
    const wr = w as unknown as Record<string, unknown>
    const er = ev as unknown as Record<string, unknown>
    const nr = n as unknown as Record<string, unknown>
    const cr = cfg as unknown as Record<string, unknown>
    watches.value = ((wr.items as unknown[]) || []).map((x) => asWatch(x as Record<string, unknown>))
    events.value = ((er.items as unknown[]) || []).map((x) => asEvent(x as Record<string, unknown>))
    notifications.value = ((nr.items as unknown[]) || []).map((x) => asNotify(x as Record<string, unknown>))
    configMasked.value = String(cr.github_token_masked ?? cr.githubTokenMasked ?? '')
  } catch {
    /* side panel optional during early wire */
  }
}

function reload() {
  void loadBoard()
  void loadSide()
}

function clearFilters() {
  query.value = ''
  language.value = null
  topic.value = ''
  onlyNew.value = false
}

function onOpen(repo: RepoItem) {
  detailRepo.value = repo
  detailOpen.value = true
}

async function onWatch(repo: RepoItem) {
  if (watchedNames.value.has(repo.full_name)) return
  try {
    await radarWatchService.add(repo.full_name)
    await loadSide()
    Notify.create({ type: 'positive', message: `已盯盘 ${repo.full_name}` })
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message })
  }
}

async function onAddWatch() {
  const full = window.prompt('输入 owner/repo')
  if (!full) return
  try {
    await radarWatchService.add(full.trim())
    await loadSide()
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message })
  }
}

async function onRemoveWatch(id: string) {
  try {
    await radarWatchService.remove(id)
    await loadSide()
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message })
  }
}

async function saveConfig() {
  savingConfig.value = true
  try {
    const cfg = await radarConfigService.set(tokenInput.value)
    configMasked.value = String((cfg as any).github_token_masked ?? (cfg as any).githubTokenMasked ?? '')
    tokenInput.value = ''
    settingsOpen.value = false
    Notify.create({ type: 'positive', message: '已保存' })
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message })
  } finally {
    savingConfig.value = false
  }
}

watch([mode, since, query, language, topic, onlyNew], () => {
  void loadBoard()
})

onMounted(() => {
  void loadBoard()
  void loadSide()
})
</script>

<style scoped lang="sass">
.radar-page
  padding: var(--space-4)
  max-width: var(--layout-max-width)
  margin: 0 auto

.radar-page__body
  gap: var(--space-4)
  align-items: stretch

.radar-page__main
  min-width: 0

.radar-page__rail
  flex: 0 0 300px
  max-width: 300px

.radar-grid
  display: grid
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr))
  gap: var(--space-4)
</style>
