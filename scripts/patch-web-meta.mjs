import fs from 'fs';
const web = process.argv[2];
const pkgPath = web + '/package.json';
const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
pkg.name = 'framework-web';
const dropDeps = ['cesium','three','@types/three','@antv/x6','@antv/x6-plugin-clipboard','@antv/x6-plugin-history','@antv/x6-plugin-keyboard','@antv/x6-plugin-selection','@antv/x6-plugin-snapline','@antv/x6-plugin-stencil','@antv/x6-plugin-transform','flv.js','novnc-core','netcdfjs','@xterm/xterm','@xterm/addon-fit','@xterm/addon-web-links','mapbox-gl','@mapbox/vector-tile','pbf','echarts-liquidfill'];
for (const d of dropDeps) {
  if (pkg.dependencies && pkg.dependencies[d]) delete pkg.dependencies[d];
  if (pkg.devDependencies && pkg.devDependencies[d]) delete pkg.devDependencies[d];
}
if (pkg.scripts && pkg.scripts.postinstall && String(pkg.scripts.postinstall).includes('cesium')) delete pkg.scripts.postinstall;
fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n');
console.log('package', pkg.name);
for (const rel of ['src/stores/auth.ts','src/services/request.ts','src/router/index.ts']) {
  const p = web + '/' + rel;
  if (!fs.existsSync(p)) continue;
  let s = fs.readFileSync(p, 'utf8');
  s = s.split('twin_access_token').join('framework_access_token');
  s = s.split('twin_refresh_token').join('framework_refresh_token');
  s = s.split('twin_chunk_reload_ts').join('framework_chunk_reload_ts');
  fs.writeFileSync(p, s);
  console.log('tokens', rel);
}
const cesium = web + '/scripts/copy-cesium.mjs';
if (fs.existsSync(cesium)) { fs.rmSync(cesium); console.log('rm cesium script'); }
