import fs from 'fs';
import path from 'path';

const root = 'E:/project/TwinMonitor/FrameWork/apps/server';
const OLD = 'twinserver';
const NEW = 'framework';

function walk(dir, acc = []) {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) {
      if (['.git', 'node_modules', 'bin'].includes(ent.name)) continue;
      walk(p, acc);
    } else acc.push(p);
  }
  return acc;
}

let changed = 0;
for (const f of walk(root)) {
  const base = path.basename(f);
  const ext = path.extname(f).toLowerCase();
  if (base === 'go.sum') continue;
  if (!['.go', '.mod', '.yaml', '.yml', '.md', '.ps1', '.json', '.toml', '.proto', '.txt'].includes(ext) && base !== 'go.mod' && base !== 'Makefile') continue;
  let s = fs.readFileSync(f, 'utf8');
  if (!s.includes(OLD)) continue;
  fs.writeFileSync(f, s.split(OLD).join(NEW));
  changed++;
}
console.log('files changed', changed);

const gm = path.join(root, 'go.mod');
let g = fs.readFileSync(gm, 'utf8');
g = g.replace(/^\/\/.*\r?\n/m, '');
g = g.replace(/module\s+\S+/, 'module framework');
g = '// framework: mono server skeleton (renamed from twinserver)\n' + g;
const utf8 = g; // already string
fs.writeFileSync(gm, g);
console.log(fs.readFileSync(gm, 'utf8').split(/\r?\n/).slice(0, 6).join('\n'));

// residual twinserver?
let residual = 0;
for (const f of walk(root)) {
  if (f.endsWith('go.sum')) continue;
  const s = fs.readFileSync(f, 'utf8');
  if (s.includes(OLD)) {
    residual++;
    console.log('residual', f);
  }
}
console.log('residual files', residual);
