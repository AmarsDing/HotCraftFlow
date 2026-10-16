#!/usr/bin/env node
// 颜色 token 门禁（EP-FE-03 补充通道）
//
// 职责：扫描 stylelint 无法覆盖的 Sass 缩进语法区域——
//   1. src/**/*.sass 文件
//   2. src/**/*.vue 的 <style lang="sass"> 块
// （css/scss 语法的 style 块与 .css/.scss 文件由 stylelint + postcss-scss 覆盖，互不重复）
//
// 规则：
//   ERROR  hex 颜色（#[0-9a-f]{3,8}）
//   ERROR  命名颜色（white/black/red/... 出现在属性值位置）
//   WARN   rgb()/hsl() 等颜色函数（存量较多，仅汇总提示；--strict-rgb 可升级为 ERROR）
//
// 豁免（无需文件白名单，全部按行形态判定）：
//   - 变量定义行：`$sass-var: ...` 或 `--css-var: ...`（token 定义是颜色的唯一合法落点）
//   - 注释行（// 与 /* */ 块注释）
//   - `// color-gate-disable-next-line` 紧急逃生（需注明原因）
//
// 退出码：存在 ERROR → 1；仅 WARN → 0。

import { readFileSync } from "node:fs";
import { globSync } from "node:fs";
import { relative, resolve } from "node:path";

const ROOT = resolve(import.meta.dirname, "..");
const SRC = resolve(ROOT, "src");
const strictRgb = process.argv.includes("--strict-rgb");

const HEX_RE = /#[0-9a-fA-F]{3,8}\b/;
const NAMED_RE =
  /:\s*(white|black|red|green|blue|gray|grey|orange|yellow|purple|pink|brown|cyan|magenta|lime|navy|teal|olive|maroon|silver|gold)\b/i;
const RGB_FN_RE = /(?<![\w-])(rgba?|hsla?|hwb|lab|lch|oklab|oklch)\(/i;
const VAR_DEF_RE = /^\s*(?:\$|--)[\w-]+\s*:/;

/** 扫描一段文本（sass 语法），startLine 为文本在文件中的起始行号（1-based） */
function scanSassText(text, startLine, file, errors, warnings) {
  const lines = text.split("\n");
  let inBlockComment = false;
  let disabled = false;
  for (let i = 0; i < lines.length; i++) {
    const lineNo = startLine + i;
    let line = lines[i];

    // 块注释状态机（sass 文件块注释少见，但 /* */ 仍合法）
    if (inBlockComment) {
      if (line.includes("*/")) inBlockComment = false;
      continue;
    }
    if (line.trim().startsWith("/*")) {
      if (!line.includes("*/")) inBlockComment = true;
      continue;
    }
    // 行注释
    const commentIdx = line.indexOf("//");
    if (commentIdx !== -1) {
      if (line.includes("color-gate-disable-next-line")) {
        disabled = true;
        continue;
      }
      line = line.slice(0, commentIdx); // 剥离行尾注释（注释里的 hex 不算）
    }
    if (!line.trim()) continue;
    if (VAR_DEF_RE.test(line)) continue; // token 定义行合法
    if (disabled) {
      disabled = false;
      continue;
    }

    const where = `${file}:${lineNo}`;
    if (HEX_RE.test(line)) {
      errors.push(`${where}  hex  ${line.trim().slice(0, 100)}`);
    } else if (NAMED_RE.test(line)) {
      errors.push(`${where}  named  ${line.trim().slice(0, 100)}`);
    } else if (RGB_FN_RE.test(line)) {
      (strictRgb ? errors : warnings).push(
        `${where}  rgb-fn  ${line.trim().slice(0, 100)}`,
      );
    }
  }
}

const errors = [];
const warnings = [];
let scannedFiles = 0;
let scannedBlocks = 0;

// 1) .sass 文件
for (const f of globSync("**/*.sass", { cwd: SRC })) {
  const abs = resolve(SRC, f);
  const text = readFileSync(abs, "utf8");
  scannedFiles++;
  scanSassText(text, 1, `src/${f}`, errors, warnings);
}

// 2) .vue 的 <style lang="sass"> 块
const STYLE_RE = /<style\b([^>]*)>/g;
for (const f of globSync("**/*.vue", { cwd: SRC })) {
  const abs = resolve(SRC, f);
  const text = readFileSync(abs, "utf8");
  for (const m of text.matchAll(STYLE_RE)) {
    if (!/lang\s*=\s*["']sass["']/.test(m[1])) continue;
    const blockStart = m.index + m[0].length;
    const closeIdx = text.indexOf("</style>", blockStart);
    if (closeIdx === -1) continue;
    const block = text.slice(blockStart, closeIdx);
    // style 标签所在行号（1-based），块内容从下一行起
    const tagLine = text.slice(0, m.index).split("\n").length;
    scannedBlocks++;
    scanSassText(block, tagLine + 1, `src/${f}`, errors, warnings);
  }
}

const rel = (s) => s;
for (const e of errors) console.error("ERROR " + rel(e));
if (warnings.length) {
  console.warn(
    `\nWARN  rgb()/hsl() 等颜色函数命中 ${warnings.length} 处（存量豁免，--strict-rgb 可升级）`,
  );
  if (process.argv.includes("--verbose")) {
    for (const w of warnings) console.warn("WARN  " + rel(w));
  }
}
console.log(
  `\ncolor-gate: ${scannedFiles} sass files + ${scannedBlocks} vue sass blocks scanned, ` +
    `${errors.length} error(s), ${warnings.length} warning(s)`,
);
process.exit(errors.length ? 1 : 0);
