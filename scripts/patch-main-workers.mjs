import fs from 'fs';
const p = process.argv[2];
let s = fs.readFileSync(p, 'utf8');
const start = s.indexOf('func newApp(');
const end = s.indexOf('\nfunc main()', start);
if (start < 0 || end < 0) throw new Error('markers');
const replacement = `func newApp(
	logger log.Logger,
	reg registry.Registrar,
	hs *http.Server,
	coreSrv *core.CoreServer,
	asynqSrv *asynqServer.Server,
	taskWorker *server.TaskWorker,
	siteMsgSub *worker.SiteMsgSubscriber,
	logRetentionWorker *worker.LogRetentionWorker,
) *kratos.App {
	// asynq 未启用时 NewAsynqServer 返回 nil，勿注册以免 nil 接口调用 Start panic。
	servers := []transport.Server{hs, coreSrv, taskWorker, siteMsgSub, logRetentionWorker}
	if asynqSrv != nil {
		servers = append(servers, asynqSrv)
	}
	return kratos.New(
		kratos.ID(os.Getenv("HOSTNAME")),
		kratos.Name(Name),
		kratos.Version("1.0.0"),
		kratos.Logger(logger),
		kratos.Server(servers...),
		kratos.Metadata(map[string]string{"weight": weight}),
		kratos.Registrar(reg),
	)
}

`;
s = s.slice(0, start) + replacement + s.slice(end);
fs.writeFileSync(p, s);
console.log('newApp slimmed');
