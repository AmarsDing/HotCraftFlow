import fs from 'fs';
const p = 'E:/project/TwinMonitor/FrameWork/apps/web/src/services/index.ts';
let s = fs.readFileSync(p, 'utf8');
s = s.replace(/const loginLogClient =/g, 'const adminLoginLogClient =');
s = s.replace(/const operationLogClient =/g, 'const adminOperationLogClient =');
s = s.replace(/const loginRestrictionClient =/g, 'const adminLoginRestrictionClient =');
s = s.replace(/const authenticationClient =/g, 'const authClient =');
fs.writeFileSync(p, s);
console.log('renamed clients');
// verify references
for (const name of ['adminLoginLogClient','adminOperationLogClient','adminLoginRestrictionClient','authClient']) {
  console.log(name, (s.match(new RegExp(name, 'g')) || []).length);
}
