import fs from 'fs';
const p = process.argv[2];
let s = fs.readFileSync(p, 'utf8');
s = s.replace(/\n\tuserV1 "twinserver\/api\/gen\/go\/user\/v1"/, '');
s = s.replace(/userV1\.Register/g, 'adminV1.Register');
fs.writeFileSync(p, s);
console.log('fixed adminV1 registers', /userV1/.test(s));
