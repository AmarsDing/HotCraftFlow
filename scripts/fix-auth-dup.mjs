import fs from 'fs';
const p = 'E:/project/TwinMonitor/FrameWork/apps/server/app/admin/internal/service/authentication_service.go';
let s = fs.readFileSync(p, 'utf8');
// collapse duplicate noop functions
const noop = `// recordUserLoginUsage skeleton no-op (license module removed).
func (s *AuthenticationService) recordUserLoginUsage(ctx context.Context, username string, resp *authenticationV1.LoginResponse) {
	_ = ctx
	_ = username
	_ = resp
}
`;
s = s.replace(/\/\/ recordUserLoginUsage skeleton no-op \(license module removed\)\.[\s\S]*?func \(s \*AuthenticationService\) recordUserLoginUsage\([\s\S]*?\n\}\r?\n\r?\n/g, '');
// insert single noop before doGrantTypeRefreshToken
if (!s.includes('recordUserLoginUsage skeleton no-op')) {
  s = s.replace(
    '// doGrantTypeRefreshToken',
    noop + '\n// doGrantTypeRefreshToken'
  );
}
fs.writeFileSync(p, s);
console.log('dup count', (s.match(/func \(s \*AuthenticationService\) recordUserLoginUsage/g) || []).length);
