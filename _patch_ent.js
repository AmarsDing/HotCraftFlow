const fs = require('fs');
const p = 'E:/project/FrameWork/apps/server/app/admin/internal/data/ent_client.go';
let c = fs.readFileSync(p, 'utf8');
const anchor = 'failed creating schema resources';
const pos = c.indexOf(anchor);
if (pos < 0) throw new Error('anchor missing');
const sub = c.slice(pos);
const idx = sub.indexOf('\t\t// ');
const abs = pos + idx;
const endMarker = '\n\tcli := entgo.NewEntClient';
const end = c.indexOf(endMarker, abs);
if (end < 0) throw new Error('cli := marker missing');
// Also need the closing brace of GetMigrate just before cli :=
// Original: business... \n\t}\n\n\tcli :=
// Keep the \n\t} that closes GetMigrate: find last \t} before end
let closeAt = c.lastIndexOf('\n\t}', end);
if (closeAt < abs) throw new Error('closing brace missing');
const replacement = '\t\t// FrameWork skeleton: skip TwinMonitor business post-migrate indexes (knowledge_base/inventory/assets/awos).\n';
c = c.slice(0, abs) + replacement + c.slice(closeAt);
fs.writeFileSync(p, c);
console.log('patched ok');
console.log(c.slice(abs - 80, abs + 200));
