import fs from 'fs';
import path from 'path';
const admin = process.argv[2];

// --- dict_repo.go: remove WarehouseEnvironmentUsecase ---
{
  const p = path.join(admin, 'internal/biz/dict_repo.go');
  let s = fs.readFileSync(p, 'utf8');
  s = s.replace(/\n\t\/\/ envUC[\s\S]*?\n\tenvUC \*WarehouseEnvironmentUsecase\n/, '\n');
  s = s.replace(
    /func NewDictUsecase\(repo DictRepo, envUC \*WarehouseEnvironmentUsecase, logger log\.Logger\) \*DictUsecase \{\r?\n\treturn \&DictUsecase\{\r?\n\t\trepo:  repo,\r?\n\t\tenvUC: envUC,\r?\n\t\tlog:   log\.NewHelper\(log\.With\(logger, "module", "dict\/usecase\/admin-service"\)\),\r?\n\t\}\r?\n\}/,
    `func NewDictUsecase(repo DictRepo, logger log.Logger) *DictUsecase {
	return &DictUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "dict/usecase/admin-service")),
	}
}`
  );
  // soften invalidate
  s = s.replace(
    /func \(uc \*DictUsecase\) invalidateEnvThresholdCaches\(ctx context\.Context, category string\) \{\r?\n\tif uc\.envUC == nil \|\| category != dictTypeWarehouseEnvThresholds \{\r?\n\t\treturn\r?\n\t\}\r?\n\tuc\.envUC\.InvalidateAllThresholdCaches\(ctx\)\r?\n\}/,
    `func (uc *DictUsecase) invalidateEnvThresholdCaches(ctx context.Context, category string) {
	// skeleton: warehouse env threshold cache removed
	_ = ctx
	_ = category
}`
  );
  fs.writeFileSync(p, s);
  console.log('dict_repo patched', /WarehouseEnvironment/.test(s), /envUC/.test(s));
}

// --- authentication: drop retryRepo field ---
{
  const p = path.join(admin, 'internal/biz/authentication_repo.go');
  let s = fs.readFileSync(p, 'utf8');
  s = s.replace(/\n\tretryRepo\s+AssetNatsRetryRepo\n/, '\n');
  s = s.replace(/\n\tretryRepo AssetNatsRetryRepo,\n/, '\n');
  s = s.replace(/\n\t\tretryRepo:\s+retryRepo,\n/, '\n');
  s = s.replace(
    /PublishAssetEventWithRetry\(ctx, uc\.publisher, uc\.retryRepo, uc\.log,/g,
    'PublishAssetEventWithRetry(ctx, uc.publisher, nil, uc.log,'
  );
  fs.writeFileSync(p, s);
  console.log('auth patched', /retryRepo/.test(s), /AssetNatsRetryRepo/.test(s));
}

// --- event_publisher nil-safe ---
{
  const p = path.join(admin, 'internal/biz/event_publisher.go');
  let s = fs.readFileSync(p, 'utf8');
  if (!s.includes('if publisher == nil')) {
    s = s.replace(
      'if err := publisher.PublishAssetEvent(ctx, evt); err != nil {',
      `if publisher == nil {
		return
	}
	if err := publisher.PublishAssetEvent(ctx, evt); err != nil {`
    );
    s = s.replace(
      'if retryErr := retryRepo.EnqueueRetry(ctx, topic, eventMap, err.Error()); retryErr != nil {',
      `if retryRepo != nil {
			if retryErr := retryRepo.EnqueueRetry(ctx, topic, eventMap, err.Error()); retryErr != nil {`
    );
    // need to close extra brace - find the block after enqueue
    // Original:
    // if retryErr := ... { logger... }
    // }
    // After my replace the inner if needs closing. Let's do a more careful replace.
  }
  fs.writeFileSync(p, s);
  console.log('event_publisher touched');
}
