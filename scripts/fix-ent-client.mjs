import fs from 'fs';
const p = process.argv[2];
let s = fs.readFileSync(p, 'utf8');
s = s.replace(/\n\t\tif err = ensureMetradarTables\([\s\S]*?\n\t\t\}\n/, '\n');
s = s.replace(/\n\t\tif err = ensureWarehouseEnvironmentPartitions\([\s\S]*?\n\t\t\}\n/, '\n');
fs.writeFileSync(p, s);
console.log('ent_client cleaned', /ensureMetradar/.test(s), /ensureWarehouse/.test(s));
