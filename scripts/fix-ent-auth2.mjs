import fs from 'fs';

// fix ent_client - comment out ensure calls
{
  const p = 'E:/project/TwinMonitor/FrameWork/apps/server/app/admin/internal/data/ent_client.go';
  let s = fs.readFileSync(p, 'utf8');
  s = s.replace(/^\t\tif err = ensureMetradarTables\([\s\S]*?^\t\t\}\r?\n/m, '');
  s = s.replace(/^\t\tif err = ensureWarehouseEnvironmentPartitions\([\s\S]*?^\t\t\}\r?\n/m, '');
  fs.writeFileSync(p, s);
  console.log('ent', /ensureMetradarTables\(/.test(s), /ensureWarehouseEnvironmentPartitions\(/.test(s));
}

// fix auth recordUserLoginUsage
{
  const p = 'E:/project/TwinMonitor/FrameWork/apps/server/app/admin/internal/service/authentication_service.go';
  let s = fs.readFileSync(p, 'utf8');
  s = s.replace(
    /\/\/ recordUserLoginUsage[\s\S]*?func \(s \*AuthenticationService\) recordUserLoginUsage\([\s\S]*?(?=\n\/\/ doGrantTypeRefreshToken)/,
    `// recordUserLoginUsage skeleton no-op (license module removed).
func (s *AuthenticationService) recordUserLoginUsage(ctx context.Context, username string, resp *authenticationV1.LoginResponse) {
	_ = ctx
	_ = username
	_ = resp
}

`
  );
  fs.writeFileSync(p, s);
  console.log('auth ok');
}
