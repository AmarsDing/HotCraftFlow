import fs from 'fs';
import path from 'path';

const web = 'E:/project/TwinMonitor/FrameWork/apps/web';
const replacements = [
  ['twin_access_token', 'framework_access_token'],
  ['twin_refresh_token', 'framework_refresh_token'],
  ['twin_chunk_reload_ts', 'framework_chunk_reload_ts'],
  ['twin_session', 'framework_session'],
  ['twin_remember', 'framework_remember'],
  ['twin_login_history', 'framework_login_history'],
  ['twin_sidebar_group_collapsed', 'framework_sidebar_group_collapsed'],
  ['twin_system_name', 'framework_system_name'],
  ['twin_op_pwd_set_', 'framework_op_pwd_set_'],
  ['twin_screen_deploy_v1', 'framework_screen_deploy_v1'],
];

function walk(dir, acc=[]) {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) {
      if (['node_modules','.quasar','dist'].includes(ent.name)) continue;
      walk(p, acc);
    } else if (/\.(ts|vue|js|mjs|json)$/.test(ent.name)) acc.push(p);
  }
  return acc;
}

let files = 0;
for (const f of walk(web)) {
  let s = fs.readFileSync(f, 'utf8');
  let orig = s;
  for (const [a,b] of replacements) s = s.split(a).join(b);
  if (s !== orig) { fs.writeFileSync(f, s); files++; console.log('patched', path.relative(web, f)); }
}
console.log('files patched', files);

// Remove obvious business leftovers that shouldn't be in skeleton
const rm = [
  'src/models/awos.ts',
  'src/types/radar.ts',
  'src/screen',
  'test/device3d-panel',
  'test/stylelint-gate',
];
for (const r of rm) {
  const p = path.join(web, r);
  if (fs.existsSync(p)) {
    fs.rmSync(p, { recursive: true, force: true });
    console.log('rm', r);
  }
}
