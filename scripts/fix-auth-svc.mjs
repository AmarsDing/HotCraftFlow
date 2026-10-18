import fs from 'fs';
const p = process.argv[2];
let s = fs.readFileSync(p, 'utf8');
s = s.replace(/\n\t\/\/ licenseUC[\s\S]*?\n\tlicenseUC \*biz\.LicenseUsecase\n/, '\n');
s = s.replace(/\n\tlicenseUC \*biz\.LicenseUsecase,\n/, '\n');
s = s.replace(/\n\t\tlicenseUC:\s+licenseUC,\n/, '\n');
s = s.replace(/\n\tif s\.licenseUC == nil \|\| resp == nil \|\| resp\.GetAccessToken\(\) == "" \{\r?\n\t\treturn\r?\n\t\}\r?\n\t[\s\S]*?LicenseUsageUserLogin[\s\S]*?\n\}/g, '\n');
// simpler line-based removal of license blocks
s = s.split(/\r?\n/).filter(line => !/licenseUC|LicenseUsage|LicenseUsecase/.test(line)).join('\n');
fs.writeFileSync(p, s);
console.log('auth service license stripped', /License/.test(s));
