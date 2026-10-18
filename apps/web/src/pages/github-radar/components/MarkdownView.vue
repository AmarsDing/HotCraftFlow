<template>
  <!-- 轻量 Markdown 渲染：标题/加粗/行内代码/代码块/列表/引用/分隔线。
       全部输入先转义后再做行内变换，防 XSS。 -->
  <!-- eslint-disable-next-line vue/no-v-html -- 输入已全量 escapeHtml 后再做行内变换，无 XSS 面 -->
  <div class="md-view" v-html="html" />
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(defineProps<{ source?: string }>(), { source: '' });

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function inline(s: string): string {
  return escapeHtml(s)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/(?<!\*)\*([^*\n]+)\*(?!\*)/g, '<em>$1</em>')
    .replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
}

function renderMarkdown(md: string): string {
  if (!md) return '';
  const lines = md.replace(/\r\n/g, '\n').split('\n');
  const out: string[] = [];
  let i = 0;
  let listType: 'ul' | 'ol' | null = null;

  const closeList = () => {
    if (listType) {
      out.push(listType === 'ul' ? '</ul>' : '</ol>');
      listType = null;
    }
  };

  while (i < lines.length) {
    const line = lines[i];

    // 代码块
    const fence = line.match(/^```(\w*)\s*$/);
    if (fence) {
      closeList();
      const buf: string[] = [];
      i++;
      while (i < lines.length && !/^```\s*$/.test(lines[i])) {
        buf.push(lines[i]);
        i++;
      }
      i++; // 跳过收尾 ```
      out.push(`<pre class="md-view__pre"><code>${escapeHtml(buf.join('\n'))}</code></pre>`);
      continue;
    }

    const trimmed = line.trim();

    if (trimmed === '') {
      closeList();
      i++;
      continue;
    }

    const heading = trimmed.match(/^(#{1,4})\s+(.*)$/);
    if (heading) {
      closeList();
      const level = heading[1].length;
      out.push(`<h${level} class="md-view__h">${inline(heading[2])}</h${level}>`);
      i++;
      continue;
    }

    if (/^(-{3,}|\*{3,}|_{3,})$/.test(trimmed)) {
      closeList();
      out.push('<hr class="md-view__hr" />');
      i++;
      continue;
    }

    const quote = trimmed.match(/^>\s?(.*)$/);
    if (quote) {
      closeList();
      out.push(`<blockquote class="md-view__quote">${inline(quote[1])}</blockquote>`);
      i++;
      continue;
    }

    const ul = trimmed.match(/^[-*+]\s+(.*)$/);
    if (ul) {
      if (listType !== 'ul') {
        closeList();
        out.push('<ul class="md-view__list">');
        listType = 'ul';
      }
      out.push(`<li>${inline(ul[1])}</li>`);
      i++;
      continue;
    }

    const ol = trimmed.match(/^\d+[.)]\s+(.*)$/);
    if (ol) {
      if (listType !== 'ol') {
        closeList();
        out.push('<ol class="md-view__list">');
        listType = 'ol';
      }
      out.push(`<li>${inline(ol[1])}</li>`);
      i++;
      continue;
    }

    // 普通段落（合并连续行）
    closeList();
    const buf = [trimmed];
    i++;
    while (i < lines.length) {
      const next = lines[i].trim();
      if (next === '' || /^(#{1,4}\s|```|[-*+]\s|\d+[.)]\s|>|(-{3,}|\*{3,}|_{3,})$)/.test(next)) {
        break;
      }
      buf.push(next);
      i++;
    }
    out.push(`<p class="md-view__p">${inline(buf.join(' '))}</p>`);
  }
  closeList();
  return out.join('\n');
}

const html = computed(() => renderMarkdown(props.source));
</script>

<style scoped lang="sass">
.md-view
  font-size: var(--text-sm)
  line-height: 1.65
  color: var(--color-text-primary)
  word-break: break-word

.md-view__h
  font-size: var(--text-base)
  font-weight: 700
  color: var(--color-text-heading, var(--color-text-primary))
  margin: var(--space-3) 0 var(--space-1)

.md-view__p
  margin: var(--space-1) 0

.md-view__list
  margin: var(--space-1) 0
  padding-left: var(--space-5)

.md-view__pre
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  border-radius: 8px
  padding: var(--space-3)
  overflow-x: auto
  font-size: var(--text-xs)
  line-height: 1.5
  margin: var(--space-2) 0

.md-view__quote
  border-left: 3px solid var(--color-accent)
  padding-left: var(--space-3)
  margin: var(--space-2) 0
  color: var(--color-text-secondary)

.md-view__hr
  border: none
  border-top: 1px solid var(--glass-border)
  margin: var(--space-3) 0

.md-view :deep(code)
  background: var(--glass-surface)
  border-radius: 4px
  padding: 1px 5px
  font-size: .92em
  font-family: var(--font-mono, ui-monospace, monospace)

.md-view :deep(a)
  color: var(--color-accent)
</style>
