import fs from 'fs';
const p = 'E:/project/TwinMonitor/FrameWork/apps/server/app/admin/internal/service/authentication_service.go';
let s = fs.readFileSync(p, 'utf8');
s = s.replace(
  /resp, err := s\.uc\.VerifyMFA\([\s\S]*?if err != nil \{\r?\n\t\treturn nil, err\r?\n\t\}\r?\n\t\/\/ MFA[\s\S]*?\r?\n\t\}\r?\n\treturn resp, nil\r?\n\}/,
  `resp, err := s.uc.VerifyMFA(ctx, req, logging.ClientIPFromContext(ctx), logging.UserAgentFromContext(ctx))
	if err != nil {
		return nil, err
	}
	s.recordUserLoginUsage(ctx, "", resp)
	return resp, nil
}`
);
fs.writeFileSync(p, s);
console.log('VerifyMFA fixed');
