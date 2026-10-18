import fs from 'fs';

// Rewrite task_worker NewTaskWorker without schedule
{
  const p = 'E:/project/TwinMonitor/FrameWork/apps/server/app/admin/internal/server/task_worker.go';
  let s = fs.readFileSync(p, 'utf8');
  s = s.replace(/\/\/ scheduleAutoGeneratePayload[\s\S]*?\ntype scheduleAutoGeneratePayload = biz\.ScheduleAutoGeneratePayload\r?\n\r?\n/, '\n');
  s = s.replace(
    /func NewTaskWorker\(asynqSrv \*asynqServer\.Server, taskUC \*biz\.TaskUsecase, sessionUC \*biz\.SessionUsecase, scheduleUC \*biz\.ScheduleUsecase, logger log\.Logger\) \*TaskWorker \{[\s\S]*?return w\r?\n\}/,
    `func NewTaskWorker(asynqSrv *asynqServer.Server, taskUC *biz.TaskUsecase, sessionUC *biz.SessionUsecase, logger log.Logger) *TaskWorker {
	l := log.NewHelper(log.With(logger, "module", "task-worker"))
	w := &TaskWorker{
		taskUC:  taskUC,
		log:     l,
		asynqOn: asynqSrv != nil,
	}
	if asynqSrv != nil {
		handler := wrapWithExecutionRecord(taskUC, biz.TaskTypeNameUserTokenCleanup,
			func(ctx context.Context) error {
				_, err := sessionUC.CleanupExpiredTokens(ctx)
				return err
			})
		if err := asynqServer.RegisterSubscriberWithCtx(asynqSrv, biz.TaskTypeNameUserTokenCleanup,
			func(ctx context.Context, _ string, _ *tokenCleanupPayload) error {
				return handler(ctx)
			}); err != nil {
			panic("register user_token_cleanup subscriber failed: " + err.Error())
		}
	}
	return w
}`
  );
  // Remove schedule task seed references if any later in file
  s = s.replace(/TaskTypeNameScheduleAutoGenerate[\s\S]{0,200}/g, (m) => {
    if (m.includes('Schedule')) return '/* schedule task removed in skeleton */';
    return m;
  });
  fs.writeFileSync(p, s);
  console.log('task_worker patched', /ScheduleUsecase/.test(s));
}

// rest whitelist
{
  const p = 'E:/project/TwinMonitor/FrameWork/apps/server/app/admin/internal/server/rest.go';
  let s = fs.readFileSync(p, 'utf8');
  s = s.replace(/\t\/\/ GetFullConfig[\s\S]*?whiteList\[adminV1\.OperationDeviceProtocolBindingServiceGetFullConfig\] = true\r?\n/, '');
  s = s.replace(/\t\/\/ 文档下载[\s\S]*?whiteList\[OperationDocumentServicePreview\] = true\r?\n/, '');
  // also english-garbled comments variants - delete by const name
  s = s.replace(/\twhiteList\[adminV1\.OperationDeviceProtocolBindingServiceGetFullConfig\] = true\r?\n/, '');
  s = s.replace(/\twhiteList\[OperationDocumentServiceDownload\] = true\r?\n/, '');
  s = s.replace(/\twhiteList\[OperationDocumentServicePreview\] = true\r?\n/, '');
  fs.writeFileSync(p, s);
  console.log('whitelist cleaned', /OperationDeviceProtocolBinding|OperationDocumentService/.test(s));
}

// payloadFloat helper for worker
fs.writeFileSync('E:/project/TwinMonitor/FrameWork/apps/server/app/admin/internal/worker/payload_helpers.go', `package worker

func payloadFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	default:
		return 0
	}
}
`);
console.log('payload helper written');
