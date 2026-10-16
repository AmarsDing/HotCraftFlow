<template>
  <!-- 轻量 Markdown 渲染器：escape-first（先转义后替换，天然 XSS 安全），零外部依赖。
       支持：# 标题 / **粗体** / *斜体* / `行内代码` / ```围栏代码块（含语言徽标与注释/字符串着色）/ - 无序列表 / 1. 有序列表 / > 引用 / --- 分隔线 -->
  <div class="md-renderer" v-html="html"></div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(defineProps<{ source?: string }>(), { source: '' });

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// 行内语法：行内代码 → 粗体 → 斜体（输入已转义，输出仍为安全 HTML；粗体先消费 ** 对，斜体仅匹配剩余单星号对）
function renderInline(escaped: string): string {
  return escaped
    .replace(/`([^`\n]+)`/g, '<code class="md-inline-code">$1</code>')
    .replace(/\*\*([^*\n]+)\*\*/g, '<strong class="md-strong">$1</strong>')
    .replace(/\*([^*\n]+)\*/g, '<em class="md-em">$1</em>');
}

// 代码块轻量着色：注释（#/-- 起）与引号字符串；输入为已转义文本
function colorizeCode(escaped: string): string {
  return escaped
    .split('\n')
    .map((line) => {
      const commentIdx = line.search(/(^|\s)(#|--)/);
      let code = line;
      let comment = '';
      if (commentIdx >= 0) {
        const m = line.slice(commentIdx).match(/(\s*)(#|--)/);
        const cut = commentIdx + (m?.[1].length ?? 0);
        code = line.slice(0, cut);
        comment = line.slice(cut);
      }
      code = code.replace(/(&quot;.*?&quot;|&#39;.*?&#39;)/g, '<span class="md-str">$1</span>');
      return comment ? `${code}<span class="md-comment">${comment}</span>` : code;
    })
    .join('\n');
}

function render(source: string): string {
  const lines = escapeHtml(source).split('\n');
  const out: string[] = [];
  let inCode = false;
  let codeLang = '';
  let codeBuf: string[] = [];
  let listType: 'ul' | 'ol' | '' = '';

  const closeList = () => {
    if (listType) {
      out.push(`</${listType}>`);
      listType = '';
    }
  };
  const flushCode = () => {
    const langBadge = codeLang ? `<span class="md-code-lang">${codeLang}</span>` : '';
    out.push(
      `<div class="md-code-block">${langBadge}<pre class="md-code"><code>${colorizeCode(codeBuf.join('\n'))}</code></pre></div>`,
    );
    codeBuf = [];
    codeLang = '';
  };

  for (const raw of lines) {
    const line = raw.replace(/\s+$/, '');
    const fence = line.match(/^```([\w-]*)\s*$/);
    if (fence) {
      if (inCode) {
        flushCode();
        inCode = false;
      } else {
        closeList();
        inCode = true;
        codeLang = fence[1] ?? '';
      }
      continue;
    }
    if (inCode) {
      codeBuf.push(raw);
      continue;
    }

    const heading = line.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      closeList();
      const level = heading[1].length;
      out.push(`<div class="md-h md-h${level}">${renderInline(heading[2])}</div>`);
      continue;
    }

    if (/^\s*(-{3,}|\*{3,})\s*$/.test(line)) {
      closeList();
      out.push('<div class="md-hr"></div>');
      continue;
    }

    const quote = line.match(/^>\s?(.*)$/);
    if (quote) {
      closeList();
      out.push(`<div class="md-quote">${renderInline(quote[1])}</div>`);
      continue;
    }

    const ul = line.match(/^\s*[-*]\s+(.+)$/);
    if (ul) {
      if (listType !== 'ul') {
        closeList();
        out.push('<ul class="md-list">');
        listType = 'ul';
      }
      out.push(`<li>${renderInline(ul[1])}</li>`);
      continue;
    }

    const ol = line.match(/^\s*\d+[.)]\s+(.+)$/);
    if (ol) {
      if (listType !== 'ol') {
        closeList();
        out.push('<ol class="md-list">');
        listType = 'ol';
      }
      out.push(`<li>${renderInline(ol[1])}</li>`);
      continue;
    }

    closeList();
    if (line.trim() === '') {
      out.push('<div class="md-gap"></div>');
    } else {
      out.push(`<div class="md-p">${renderInline(line)}</div>`);
    }
  }
  if (inCode) flushCode();
  closeList();
  return out.join('');
}

const html = computed(() => render(props.source || ''));
</script>

<style lang="sass">
// 非 scoped：v-html 注入内容需要全局选择器；类名统一 md- 前缀防碰撞
.md-renderer
  // 代码块压暗混合基底（仅 dark 覆盖层消费，恒黑）
  --md-mix-black: #000
  font-size: 13px
  line-height: 1.75
  color: var(--color-text-primary)
  word-break: break-word

  .md-p
    margin: 0

  .md-gap
    height: 8px

  .md-h
    font-weight: 700
    color: var(--color-text-heading)
    margin: 10px 0 4px

  .md-h1
    font-size: 17px
  .md-h2
    font-size: 15px
  .md-h3
    font-size: 14px
  .md-h4
    font-size: 13px

  .md-h:first-child
    margin-top: 0

  .md-strong
    font-weight: 700
    color: var(--color-text-heading)

  .md-em
    font-style: italic

  .md-inline-code
    font-family: monospace
    font-size: 12px
    padding: 1px 6px
    border-radius: 5px
    background: color-mix(in srgb, var(--color-accent) 10%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-accent) 22%, transparent)
    color: var(--color-accent)

  .md-list
    margin: 2px 0
    padding-left: 22px

    li
      margin: 2px 0

  .md-quote
    margin: 4px 0
    padding: 4px 12px
    border-left: 3px solid color-mix(in srgb, var(--color-accent) 45%, transparent)
    background: color-mix(in srgb, var(--color-accent) 6%, transparent)
    border-radius: 0 6px 6px 0
    color: var(--color-text-secondary)

  .md-hr
    height: 1px
    margin: 10px 0
    background: var(--glass-border)

  .md-code-block
    position: relative
    margin: 6px 0
    border-radius: 8px
    border: 1px solid var(--glass-border)
    background: var(--glass-surface)
    overflow: hidden

  .md-code-lang
    position: absolute
    top: 6px
    right: 10px
    font-size: 10px
    font-weight: 700
    letter-spacing: 0.08em
    text-transform: uppercase
    color: var(--color-text-tertiary)
    pointer-events: none

  .md-code
    margin: 0
    padding: 12px 14px
    font-family: monospace
    font-size: 12px
    line-height: 1.65
    white-space: pre-wrap
    word-break: break-all
    color: var(--color-text-primary)

  .md-str
    color: var(--q-positive)

  .md-comment
    color: var(--color-text-tertiary)
    font-style: italic

body.body--dark
  .md-renderer .md-code-block
    background: color-mix(in srgb, var(--glass-surface) 70%, var(--md-mix-black) 8%)
</style>
