<template>
  <!-- 轻量 Markdown 渲染器：escape-first（先转义后替换，天然 XSS 安全），零外部依赖。
       支持：# 标题 / **粗体** / *斜体* / `行内代码` / [链接](URL 白名单)/ ![图片](仅 https?) /
       ```围栏代码块（含语言徽标与注释/字符串着色）/ - 无序列表 / 1. 有序列表 / > 引用（连续行合并）/
       --- 分隔线 / | 表格 |
       URL 纪律：链接仅放行 https?://、mailto:、#锚点、/相对路径；图片仅放行 https?://。
       javascript:/data: 等一律按原文本展示（escape 后已无注入面，仅是冗余兜底）。 -->
  <!-- eslint-disable-next-line vue/no-v-html -- escape-first：先全文转义再做受控替换，输出无注入面 -->
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

/** 链接 scheme 白名单（图片比链接更严，仅放行 https?） */
function isSafeLink(url: string): boolean {
  return /^(https?:\/\/|mailto:|#|\/)/i.test(url.trim());
}
function isSafeImage(url: string): boolean {
  return /^https?:\/\//i.test(url.trim());
}

// 行内语法：行内代码 → 图片 → 链接 → 粗体 → 斜体（输入已转义，输出仍为安全 HTML）
function renderInline(escaped: string): string {
  return escaped
    .replace(/`([^`\n]+)`/g, '<code class="md-inline-code">$1</code>')
    .replace(/!\[([^\]\n]*)\]\(([^)\n]+)\)/g, (m, alt: string, url: string) => {
      if (!isSafeImage(url)) return m;
      return `<img class="md-img" src="${url.trim()}" alt="${alt}" loading="lazy" />`;
    })
    .replace(/\[([^\]\n]+)\]\(([^)\n]+)\)/g, (m, text: string, url: string) => {
      if (!isSafeLink(url)) return m;
      return `<a class="md-link" href="${url.trim()}" target="_blank" rel="noopener noreferrer">${text}</a>`;
    })
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
  let quoteBuf: string[] = [];

  const closeList = () => {
    if (listType) {
      out.push(`</${listType}>`);
      listType = '';
    }
  };
  const flushQuote = () => {
    if (quoteBuf.length) {
      out.push(`<div class="md-quote">${quoteBuf.map((q) => renderInline(q)).join('<br>')}</div>`);
      quoteBuf = [];
    }
  };
  const closeBlocks = () => {
    closeList();
    flushQuote();
  };
  const flushCode = () => {
    const langBadge = codeLang ? `<span class="md-code-lang">${codeLang}</span>` : '';
    out.push(
      `<div class="md-code-block">${langBadge}<pre class="md-code"><code>${colorizeCode(codeBuf.join('\n'))}</code></pre></div>`,
    );
    codeBuf = [];
    codeLang = '';
  };

  const isTableRow = (l: string) => /^\s*\|.*\|\s*$/.test(l);
  const isTableSep = (l: string) => /^\s*\|?\s*:?-{1,}[\s:|-]*\|?[\s:|-]*$/.test(l);
  const splitCells = (l: string) =>
    l
      .replace(/^\s*\||\|\s*$/g, '')
      .split('|')
      .map((c) => c.trim());
  const tableHtml = (header: string[], rows: string[][]): string => {
    const th = header.map((c) => `<th>${renderInline(c)}</th>`).join('');
    const trs = rows.map((r) => `<tr>${r.map((c) => `<td>${renderInline(c)}</td>`).join('')}</tr>`).join('');
    return `<table class="md-table"><thead><tr>${th}</tr></thead><tbody>${trs}</tbody></table>`;
  };

  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i];
    const line = raw.replace(/\s+$/, '');
    const fence = line.match(/^```([\w-]*)\s*$/);
    if (fence) {
      if (inCode) {
        flushCode();
        inCode = false;
      } else {
        closeBlocks();
        inCode = true;
        codeLang = fence[1] ?? '';
      }
      continue;
    }
    if (inCode) {
      codeBuf.push(raw);
      continue;
    }

    // 表格：表头行 + 下一行是分隔行（| --- | --- |）
    if (isTableRow(line) && i + 1 < lines.length) {
      const next = lines[i + 1].replace(/\s+$/, '');
      if (isTableSep(next)) {
        closeBlocks();
        const header = splitCells(line);
        i++; // 消费分隔行
        const rows: string[][] = [];
        while (i + 1 < lines.length && isTableRow(lines[i + 1].replace(/\s+$/, ''))) {
          i++;
          rows.push(splitCells(lines[i].replace(/\s+$/, '')));
        }
        out.push(tableHtml(header, rows));
        continue;
      }
    }

    const heading = line.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      closeBlocks();
      const level = heading[1].length;
      out.push(`<div class="md-h md-h${level}">${renderInline(heading[2])}</div>`);
      continue;
    }

    if (/^\s*(-{3,}|\*{3,})\s*$/.test(line)) {
      closeBlocks();
      out.push('<div class="md-hr"></div>');
      continue;
    }

    const quote = line.match(/^>\s?(.*)$/);
    if (quote) {
      closeList();
      quoteBuf.push(quote[1]);
      continue;
    }

    const ul = line.match(/^\s*[-*]\s+(.+)$/);
    if (ul) {
      flushQuote();
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
      flushQuote();
      if (listType !== 'ol') {
        closeList();
        out.push('<ol class="md-list">');
        listType = 'ol';
      }
      out.push(`<li>${renderInline(ol[1])}</li>`);
      continue;
    }

    closeBlocks();
    if (line.trim() === '') {
      out.push('<div class="md-gap"></div>');
    } else {
      out.push(`<div class="md-p">${renderInline(line)}</div>`);
    }
  }
  if (inCode) flushCode();
  closeBlocks();
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

  .md-link
    color: var(--color-accent)
    text-decoration: none
    &:hover
      text-decoration: underline

  .md-img
    display: block
    max-width: 100%
    margin: 6px 0
    border-radius: 8px
    border: 1px solid var(--glass-border)

  .md-table
    border-collapse: collapse
    margin: 6px 0
    font-size: 12px
    th, td
      border: 1px solid var(--glass-border)
      padding: 4px 10px
      text-align: left
    th
      background: var(--glass-surface)
      font-weight: 700
      color: var(--color-text-heading)
    td
      color: var(--color-text-primary)

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
