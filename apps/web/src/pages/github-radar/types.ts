/** Local DTO until buf generate → services/kratos. Do not hand-edit kratos gen. */

export type LanguageStat = {
  name: string;
  bytes?: number;
  percent?: number;
};

export type RepoItem = {
  id: string;
  full_name: string;
  description: string;
  language: string;
  topics: string[];
  stars: number;
  forks: number;
  rank: number;
  stars_delta: number;
  is_new: boolean;
  html_url: string;
  sparkline: number[];
  // ── v2 ──
  license?: string;
  license_spdx?: string;
  homepage?: string;
  open_issues?: number;
  watchers?: number;
  contributors?: string[];
  created_at?: string;
  updated_at?: string;
  pushed_at?: string;
  clone_url?: string;
  ssh_url?: string;
  zip_url?: string;
  default_branch?: string;
  languages?: LanguageStat[];
  readme_excerpt?: string;
};

export type WatchItem = {
  id: string;
  full_name: string;
  stars: number;
  stars_delta: number;
  unread_count?: number;
  sparkline?: number[];
  html_url?: string;
};

export type WatchEvent = {
  id: string;
  full_name: string;
  kind: string;
  message: string;
  created_at: string;
  read: boolean;
};

export type RadarNotification = {
  id: string;
  title: string;
  body: string;
  kind: string;
  created_at: string;
  read: boolean;
};

export type AIInsight = {
  full_name: string;
  level: 'brief' | 'summary' | 'deep';
  content_markdown: string;
  model: string;
  generated_at: string;
  cached: boolean;
  duration_ms: number;
  error: string;
};

export type TreeNode = {
  path: string;
  type: 'blob' | 'tree';
  size: number;
};

export function splitFullName(full: string) {
  const i = full.indexOf('/');
  if (i < 0) return { owner: '', name: full };
  return { owner: full.slice(0, i), name: full.slice(i + 1) };
}

export function abbrevCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

export function exactCount(n: number): string {
  return n.toLocaleString('en-US');
}

/** 相对时间：刚刚 / N 分钟前 / N 小时前 / N 天前 / 日期 */
export function relTime(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  const diff = Date.now() - d.getTime();
  if (diff < 60_000) return '刚刚';
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`;
  if (diff < 30 * 86_400_000) return `${Math.floor(diff / 86_400_000)} 天前`;
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** GitHub 官方语言色表（inline style 承载，规避 sass hex 门禁） */
const LANG_COLORS: Record<string, string> = {
  TypeScript: '#3178c6',
  JavaScript: '#f1e05a',
  Python: '#3572A5',
  Go: '#00ADD8',
  Rust: '#dea584',
  Java: '#b07219',
  'C++': '#f34b7d',
  C: '#555555',
  'C#': '#178600',
  Ruby: '#701516',
  PHP: '#4F5D95',
  Swift: '#F05138',
  Kotlin: '#A97BFF',
  Dart: '#00B4AB',
  HTML: '#e34c26',
  CSS: '#563d7c',
  SCSS: '#c6538c',
  Vue: '#41b883',
  Shell: '#89e051',
  Lua: '#000080',
  Perl: '#0298c3',
  Scala: '#c22d40',
  Haskell: '#5e5086',
  Elixir: '#6e4a7e',
  Erlang: '#B83998',
  Clojure: '#db5855',
  'Objective-C': '#438eff',
  Assembly: '#6E4C13',
  PowerShell: '#012456',
  Makefile: '#427819',
  Dockerfile: '#384d54',
  Zig: '#ec915c',
  Jupyter: '#DA5B0B',
  Notebook: '#DA5B0B',
  MDX: '#fcb32c',
  Markdown: '#083fa1',
};

export function langColor(lang?: string): string {
  if (!lang) return 'var(--color-text-tertiary, var(--color-text-secondary))';
  return LANG_COLORS[lang] || '#8b949e';
}

export function formatBytes(n: number): string {
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  if (n >= 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${n} B`;
}

// label/desc 为中文 fallback；展示层（RepoInsight）以 i18n githubRadar.insightLevel.* 覆盖实现多语言。
export const INSIGHT_LEVELS = [
  { value: 'brief', label: '一句话', icon: 'bolt', desc: '50 字内极速概括' },
  { value: 'summary', label: '速览', icon: 'summarize', desc: '是什么 / 技术栈 / 亮点 / 场景 / 热度' },
  { value: 'deep', label: '深度', icon: 'psychology', desc: '架构推测 / 优劣 / 社区健康度 / 上手建议' },
] as const;
