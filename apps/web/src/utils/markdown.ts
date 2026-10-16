// 迷你 Markdown 渲染器：报表预览/在线文档/告警源接入指南专用。
// 背景：预览内容是后端 render/markdown.go 生成的受限 GFM 子集
//（#/##/### 标题、- 列表、| 表格 |、段落、**加粗**、\| 转义）；
// 接入指南（monitoralarm alert_source.go GetGuide）额外用到 1. 有序列表、``` 围栏代码块、`行内代码`。
// 引入 marked/markdown-it 体积不划算（§12.6 禁未经评估依赖），故按行级解析自绘。
// 安全：所有文本节点先 HTML 转义再套标签，设备名等不可信输入不会注入。

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// 行内格式：**加粗**、`行内代码`、GFM 表格转义符 \| 还原
function inline(s: string): string {
  return escapeHtml(s)
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\\\|/g, '|');
}

function isTableRow(line: string): boolean {
  return line.startsWith('|') && line.endsWith('|');
}

function isTableSeparator(line: string): boolean {
  return /^\|[\s:-]+(\|[\s:-]+)*\|?$/.test(line);
}

function splitTableRow(line: string): string[] {
  return line
    .slice(1, -1)
    .split(/(?<!\\)\|/)
    .map((c) => c.trim());
}

/**
 * 受限 GFM → HTML。支持：h1-h3、无序/有序列表、表格、围栏代码块、段落、**加粗**、`行内代码`。
 * 不支持的语法按段落文本原样转义输出，保证不丢内容。
 */
export function miniMarkdownToHtml(md: string): string {
  const lines = (md || '').split(/\r?\n/);
  const out: string[] = [];
  let i = 0;
  let inList: 'ul' | 'ol' | null = null;
  let inCode = false;
  let codeBuf: string[] = [];

  const closeList = () => {
    if (inList) {
      out.push(inList === 'ol' ? '</ol>' : '</ul>');
      inList = null;
    }
  };

  while (i < lines.length) {
    const line = lines[i];
    const trimmed = line.trim();

    // 围栏代码块：``` 开/合，块内不做任何语法解析（仅整体转义）
    if (trimmed.startsWith('```')) {
      if (inCode) {
        out.push(`<pre><code>${escapeHtml(codeBuf.join('\n'))}</code></pre>`);
        inCode = false;
        codeBuf = [];
      } else {
        closeList();
        inCode = true;
        codeBuf = [];
      }
      i += 1;
      continue;
    }
    if (inCode) {
      codeBuf.push(line);
      i += 1;
      continue;
    }

    if (!trimmed) {
      closeList();
      i += 1;
      continue;
    }

    // 表格：连续 table 行聚合成 <table>（首行表头，次行分隔符跳过）
    if (isTableRow(trimmed)) {
      closeList();
      const rows: string[][] = [];
      while (i < lines.length && isTableRow(lines[i].trim())) {
        if (!isTableSeparator(lines[i].trim())) rows.push(splitTableRow(lines[i].trim()));
        i += 1;
      }
      if (rows.length > 0) {
        const [head, ...body] = rows;
        out.push(
          '<table><thead><tr>' +
            head.map((c) => `<th>${inline(c)}</th>`).join('') +
            '</tr></thead><tbody>' +
            body.map((r) => '<tr>' + r.map((c) => `<td>${inline(c)}</td>`).join('') + '</tr>').join('') +
            '</tbody></table>',
        );
      }
      continue;
    }

    const h = /^(#{1,3})\s+(.*)$/.exec(trimmed);
    if (h) {
      closeList();
      const level = h[1].length;
      out.push(`<h${level}>${inline(h[2])}</h${level}>`);
      i += 1;
      continue;
    }

    const li = /^[-*]\s+(.*)$/.exec(trimmed);
    if (li) {
      if (inList !== 'ul') {
        closeList();
        out.push('<ul>');
        inList = 'ul';
      }
      out.push(`<li>${inline(li[1])}</li>`);
      i += 1;
      continue;
    }

    const ol = /^\d+[.)]\s+(.*)$/.exec(trimmed);
    if (ol) {
      if (inList !== 'ol') {
        closeList();
        out.push('<ol>');
        inList = 'ol';
      }
      out.push(`<li>${inline(ol[1])}</li>`);
      i += 1;
      continue;
    }

    closeList();
    out.push(`<p>${inline(trimmed)}</p>`);
    i += 1;
  }
  // 未闭合围栏兜底：按已累积内容输出，避免整段丢失
  if (inCode) {
    out.push(`<pre><code>${escapeHtml(codeBuf.join('\n'))}</code></pre>`);
  }
  closeList();
  return out.join('');
}
