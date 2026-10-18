import fs from 'fs';
const p = process.argv[2];
let s = fs.readFileSync(p, 'utf8');
s = s.replace(/\n\t"twinserver\/app\/admin\/internal\/data\/ent\/monitorasset"\r?\n/, '\n');
s = s.replace(/\n\t"twinserver\/app\/admin\/internal\/data\/ent\/schedulerule"\r?\n/, '\n');
s = s.replace(
  /func \(r \*dictRepo\) CountAssetSubsystemRefs\(ctx context\.Context, entryID uint32\) \(int, error\) \{[\s\S]*?\n\}/,
  `func (r *dictRepo) CountAssetSubsystemRefs(ctx context.Context, entryID uint32) (int, error) {
	_ = ctx
	_ = entryID
	return 0, nil
}`
);
s = s.replace(
  /func \(r \*dictRepo\) ListHolidayDictCodes\(ctx context\.Context\) \(\[\]string, error\) \{[\s\S]*?\n\}/,
  `func (r *dictRepo) ListHolidayDictCodes(ctx context.Context) ([]string, error) {
	_ = ctx
	return nil, nil
}`
);
fs.writeFileSync(p, s);
console.log('dict_category stubs', /monitorasset/.test(s), /schedulerule/.test(s));
