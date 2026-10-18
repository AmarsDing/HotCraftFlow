<template>
  <!-- 源码浏览器：文件树 + 代码查看（轻量语法着色） -->
  <div class="source-browser">
    <div v-if="loadingTree" class="source-browser__loading">
      <q-spinner-dots size="28px" color="primary" />
      <span class="text-caption q-ml-sm" style="color: var(--color-text-secondary)">{{
        t('githubRadar.loadingTree')
      }}</span>
    </div>

    <div v-else class="source-browser__layout">
      <div class="source-browser__tree">
        <div class="source-browser__tree-head">
          <span class="text-caption text-weight-medium">{{ repoName }}</span>
          <q-badge v-if="defaultBranch" outline color="grey-7" class="q-ml-xs">{{ defaultBranch }}</q-badge>
          <q-space />
          <q-btn flat dense round size="sm" icon="refresh" :aria-label="t('githubRadar.refreshTree')" @click="loadTree">
            <q-tooltip>{{ t('githubRadar.refreshTree') }}</q-tooltip>
          </q-btn>
        </div>
        <q-banner v-if="truncated" dense class="bg-warning text-white q-ma-sm">
          {{ t('githubRadar.treeTruncated') }}
        </q-banner>
        <div class="source-browser__tree-body scroll">
          <div
            v-for="node in treeNodes"
            :key="node.path"
            class="source-browser__node"
            :class="{ 'is-active': activePath === node.path }"
            role="button"
            tabindex="0"
            @click="onNodeClick(node)"
            @keydown.enter="onNodeClick(node)"
          >
            <q-icon
              :name="node.type === 'tree' ? (isOpen(node) ? 'folder_open' : 'folder') : 'insert_drive_file'"
              size="15px"
              class="q-mr-xs"
            />
            <span class="ellipsis">{{ node.name }}</span>
          </div>
        </div>
      </div>

      <div class="source-browser__viewer">
        <div v-if="!activePath" class="dash-card__empty app-empty-state-center--sm">
          <q-icon name="code" size="36px" />
          <div class="text-body2 q-mt-sm">{{ t('githubRadar.pickFile') }}</div>
          <div class="text-caption" style="color: var(--color-text-secondary)">
            {{ t('githubRadar.treeHint') }}
          </div>
        </div>

        <template v-else>
          <div class="source-browser__viewer-head">
            <span class="text-caption ellipsis">{{ activePath }}</span>
            <q-badge v-if="fileSize" outline color="grey-7" class="q-ml-xs">{{ fileSize }}</q-badge>
            <q-space />
            <q-btn
              v-if="fileHtmlUrl"
              flat
              dense
              round
              size="sm"
              icon="open_in_new"
              :aria-label="t('githubRadar.openOnGitHub')"
              :href="fileHtmlUrl"
              target="_blank"
              tag="a"
            >
              <q-tooltip>{{ t('githubRadar.openOnGitHub') }}</q-tooltip>
            </q-btn>
          </div>

          <div v-if="loadingContent" class="source-browser__loading">
            <q-spinner size="24px" color="primary" />
          </div>

          <q-banner v-else-if="contentError" dense rounded class="q-ma-sm bg-negative text-white">
            {{ contentError }}
          </q-banner>

          <q-banner v-else-if="!isTextFile" dense rounded class="q-ma-sm bg-info text-white">
            {{ t('githubRadar.binaryFile', { size: fileSize || t('githubRadar.unknownSize') }) }}
            <template #action>
              <q-btn
                v-if="fileDownloadUrl"
                flat
                dense
                :label="t('githubRadar.download')"
                :href="fileDownloadUrl"
                target="_blank"
                tag="a"
              />
            </template>
          </q-banner>

          <!-- eslint-disable vue/no-v-html -- highlight() 内已 escapeHtml，无 XSS 面 -->
          <pre v-else class="source-browser__code scroll"><code
            v-for="(line, idx) in codeLines"
            :key="idx"
            class="source-browser__line"
          ><span class="source-browser__ln">{{ idx + 1 }}</span><span class="source-browser__lc" v-html="highlight(line)" /></code></pre>
          <!-- eslint-enable vue/no-v-html -->
          <div v-if="contentTruncated" class="text-caption q-pa-sm" style="color: var(--color-warning)">
            {{ t('githubRadar.fileTruncated') }}
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import { radarRepoService } from '../../../services';
import { formatBytes, splitFullName, type TreeNode } from '../types';

const { t } = useI18n();

const props = defineProps<{ fullName: string }>();

interface FlatNode extends TreeNode {
  name: string;
}

const loadingTree = ref(false);
const truncated = ref(false);
const defaultBranch = ref('');
const flatNodes = ref<FlatNode[]>([]);
const openDirs = ref<Set<string>>(new Set());
const activePath = ref('');
const loadingContent = ref(false);
const contentError = ref('');
const isTextFile = ref(true);
const contentTruncated = ref(false);
const fileHtmlUrl = ref('');
const fileDownloadUrl = ref('');
const fileSize = ref('');
const codeLines = ref<string[]>([]);

const repoName = computed(() => splitFullName(props.fullName).name || props.fullName);

// 只展开根目录：过滤掉深层路径（目录取第一段）
const treeNodes = computed(() => {
  const out: FlatNode[] = [];
  for (const n of flatNodes.value) {
    const depth = n.path.split('/').length;
    if (depth === 1) {
      out.push(n);
    } else if (openDirs.value.has(n.path.split('/')[0])) {
      // 展示二级（更深层折叠为目录项）
      const second = n.path.split('/').slice(0, 2).join('/');
      if (!out.some((x) => x.path === second)) {
        const isDir = n.type === 'tree' || depth > 2;
        out.push({ ...n, path: second, name: second.split('/').slice(1).join('/'), type: isDir ? 'tree' : 'blob' });
      }
    }
  }
  return out;
});

function isOpen(node: FlatNode): boolean {
  return openDirs.value.has(node.path.split('/')[0]);
}

async function loadTree() {
  loadingTree.value = true;
  try {
    const res = await radarRepoService.getTree(props.fullName);
    defaultBranch.value = res.defaultBranch || '';
    truncated.value = !!res.truncated;
    flatNodes.value = (res.nodes || []).map((n) => {
      const path = n.path || '';
      return {
        path,
        type: (n.type || 'blob') as 'blob' | 'tree',
        size: n.size || 0,
        name: path.split('/').pop() || path,
      };
    });
    // 默认展开首层目录中的第一个（无操作，保持根层）
  } catch (e) {
    Notify.create({ type: 'negative', message: t('githubRadar.treeLoadFailed', { msg: (e as Error).message }) });
  } finally {
    loadingTree.value = false;
  }
}

async function onNodeClick(node: FlatNode) {
  if (node.type === 'tree') {
    const root = node.path.split('/')[0];
    const next = new Set(openDirs.value);
    if (next.has(root)) next.delete(root);
    else next.add(root);
    openDirs.value = next;
    return;
  }
  activePath.value = node.path;
  loadingContent.value = true;
  contentError.value = '';
  isTextFile.value = true;
  contentTruncated.value = false;
  codeLines.value = [];
  fileHtmlUrl.value = '';
  fileDownloadUrl.value = '';
  fileSize.value = node.size ? formatBytes(node.size) : '';
  try {
    const res = await radarRepoService.getContent(props.fullName, node.path, defaultBranch.value);
    fileHtmlUrl.value = res.htmlUrl || '';
    fileDownloadUrl.value = res.downloadUrl || '';
    if (res.size) fileSize.value = formatBytes(res.size);
    contentTruncated.value = !!res.truncated;
    if (res.encoding === 'binary') {
      isTextFile.value = false;
      return;
    }
    isTextFile.value = true;
    codeLines.value = (res.content || '').replace(/\n$/, '').split('\n');
  } catch (e) {
    contentError.value = (e as Error).message || t('githubRadar.fileLoadFailed');
  } finally {
    loadingContent.value = false;
  }
}

// ── 轻量语法着色（注释/字符串/数字/关键字），颜色走平台 token ──
const KEYWORDS = new Set([
  'const',
  'let',
  'var',
  'function',
  'return',
  'if',
  'else',
  'for',
  'while',
  'class',
  'new',
  'import',
  'from',
  'export',
  'default',
  'await',
  'async',
  'def',
  'self',
  'None',
  'True',
  'False',
  'func',
  'package',
  'struct',
  'interface',
  'type',
  'go',
  'defer',
  'range',
  'map',
  'chan',
  'public',
  'private',
  'static',
  'void',
  'int',
  'string',
  'bool',
  'fn',
  'impl',
  'pub',
  'use',
  'try',
  'catch',
  'throw',
  'switch',
  'case',
  'break',
  'continue',
  'this',
  'extends',
  'super',
]);

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function highlight(line: string): string {
  if (!line) return ' ';
  // 注释（整行或行尾简判）
  const commentMatch = line.match(/^(\s*)(\/\/|#)(.*)$/);
  if (commentMatch) {
    return `<span class="tok-c">${escapeHtml(line)}</span>`;
  }
  // 粗分为 token：字符串优先
  const parts = line.split(/("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`(?:[^`\\]|\\.)*`)/g);
  return parts
    .map((part) => {
      if (!part) return '';
      if (/^["'`]/.test(part)) {
        return `<span class="tok-s">${escapeHtml(part)}</span>`;
      }
      // 数字与关键字
      return escapeHtml(part)
        .replace(/\b(\d+(?:\.\d+)?)\b/g, '<span class="tok-n">$1</span>')
        .replace(/\b([A-Za-z_]{2,})\b/g, (m) => (KEYWORDS.has(m) ? `<span class="tok-k">${m}</span>` : m));
    })
    .join('');
}

onMounted(loadTree);
</script>

<style scoped lang="sass">
.source-browser
  height: 100%
  min-height: 380px

.source-browser__loading
  display: flex
  align-items: center
  justify-content: center
  padding: var(--space-6)

.source-browser__layout
  display: grid
  grid-template-columns: 220px 1fr
  gap: var(--space-3)
  height: 100%
  @media (max-width: 700px)
    grid-template-columns: 1fr

.source-browser__tree
  border: 1px solid var(--glass-border)
  border-radius: 10px
  overflow: hidden
  display: flex
  flex-direction: column
  max-height: 420px

.source-browser__tree-head
  display: flex
  align-items: center
  padding: var(--space-2) var(--space-3)
  border-bottom: 1px solid var(--glass-border)

.source-browser__tree-body
  overflow: auto
  padding: var(--space-1)

.source-browser__node
  display: flex
  align-items: center
  padding: 3px var(--space-2)
  border-radius: 6px
  font-size: var(--text-xs)
  color: var(--color-text-secondary)
  cursor: pointer
  &:hover
    background: var(--glass-surface-hover)
    color: var(--color-text-primary)
  &.is-active
    background: var(--color-accent-soft)
    color: var(--color-accent)

.source-browser__viewer
  border: 1px solid var(--glass-border)
  border-radius: 10px
  overflow: hidden
  display: flex
  flex-direction: column
  max-height: 420px

.source-browser__viewer-head
  display: flex
  align-items: center
  padding: var(--space-2) var(--space-3)
  border-bottom: 1px solid var(--glass-border)

.source-browser__code
  margin: 0
  padding: var(--space-2) 0
  overflow: auto
  font-size: var(--text-xs)
  line-height: 1.55
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace
  background: var(--glass-surface)

.source-browser__line
  display: flex
  min-width: max-content

.source-browser__ln
  flex: 0 0 44px
  text-align: right
  padding-right: var(--space-3)
  color: var(--color-text-tertiary, var(--color-text-secondary))
  user-select: none

.source-browser__lc
  white-space: pre

.source-browser :deep(.tok-k)
  color: var(--color-accent)
  font-weight: 600

.source-browser :deep(.tok-s)
  color: var(--color-success)

.source-browser :deep(.tok-n)
  color: var(--color-warning)

.source-browser :deep(.tok-c)
  color: var(--color-text-tertiary, var(--color-text-secondary))
  font-style: italic
</style>
