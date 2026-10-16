package logging

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/http"

	"github.com/mileusna/useragent"

	"github.com/tx7do/go-utils/timeutil"
	"github.com/tx7do/go-utils/trans"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 异步落库队列参数：操作日志每请求一条，属高频写入，同步落库会拖慢响应关键路径。
// 改为投递到有缓冲 channel，由单个后台 goroutine 按"每 500ms 或累积 50 条"批量落库。
const (
	logQueueCapacity  = 1024
	logFlushBatchSize = 50
	logFlushInterval  = 500 * time.Millisecond
	logWriteTimeout   = 5 * time.Second
)

// logWriteTask 待落库的日志条目（操作日志与登录日志互斥，二选一）。
type logWriteTask struct {
	operationLog *adminV1.AdminOperationLog
	loginLog     *adminV1.AdminLoginLog
}

// asyncLogWriter 日志异步落库器：非阻塞投递 + 单后台 goroutine 批量 flush。
// 生命周期跟随进程（中间件随服务启动创建一次），服务退出不强制 drain。
type asyncLogWriter struct {
	options

	queue   chan logWriteTask
	log     *log.Helper
	dropped atomic.Uint64
}

func newAsyncLogWriter(op options) *asyncLogWriter {
	w := &asyncLogWriter{
		options: op,
		queue:   make(chan logWriteTask, logQueueCapacity),
		log:     log.NewHelper(log.With(op.logger, "module", "logging.middleware")),
	}
	go w.run()
	return w
}

// enqueue 非阻塞投递；队列满时丢弃并计数告警（不阻塞请求关键路径）。
func (w *asyncLogWriter) enqueue(task logWriteTask) {
	select {
	case w.queue <- task:
	default:
		dropped := w.dropped.Add(1)
		if dropped == 1 || dropped%100 == 0 {
			w.log.Warnf("log write queue is full, dropping log entry (total dropped: %d)", dropped)
		}
	}
}

// run 后台 flush 循环：累积 50 条或每 500ms 触发一次批量落库。
func (w *asyncLogWriter) run() {
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()

	batch := make([]logWriteTask, 0, logFlushBatchSize)
	flush := func() {
		for _, task := range batch {
			w.write(task)
		}
		batch = batch[:0]
	}

	for {
		select {
		case task := <-w.queue:
			batch = append(batch, task)
			if len(batch) >= logFlushBatchSize {
				flush()
			}
		case <-ticker.C:
			if len(batch) > 0 {
				flush()
			}
		}
	}
}

// write 逐条落库。请求 ctx 在响应返回后即被取消，落库须使用独立超时 ctx。
func (w *asyncLogWriter) write(task logWriteTask) {
	ctx, cancel := context.WithTimeout(context.Background(), logWriteTimeout)
	defer cancel()

	if task.operationLog != nil && w.writeOperationLogFunc != nil {
		if err := w.writeOperationLogFunc(ctx, task.operationLog); err != nil {
			w.log.Warnf("write operation log failed: %s", err.Error())
		}
	}
	if task.loginLog != nil && w.writeLoginLogFunc != nil {
		if err := w.writeLoginLogFunc(ctx, task.loginLog); err != nil {
			w.log.Warnf("write login log failed: %s", err.Error())
		}
	}
}

// Server is an server logging middleware.
func Server(opts ...Option) middleware.Middleware {
	op := options{}
	for _, o := range opts {
		o(&op)
	}
	if op.logger == nil {
		op.logger = log.DefaultLogger
	}

	writer := newAsyncLogWriter(op)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (reply interface{}, err error) {
			startTime := time.Now()

			reply, err = handler(ctx, req)

			// 统计耗时
			latency := time.Since(startTime).Seconds()

			var operationLogData *adminV1.AdminOperationLog
			var loginLogData *adminV1.AdminLoginLog

			if tr, ok := transport.FromServerContext(ctx); ok {
				var htr *http.Transport
				if htr, ok = tr.(*http.Transport); ok {
					loginLogData = fillLoginLog(htr)
					operationLogData = fillOperationLog(htr)
				}
			}

			// 获取错误码和是否成功
			statusCode, reason, success := getStatusCode(err)

			if operationLogData != nil {
				operationLogData.CostTime = timeutil.Float64ToDurationpb(latency)
				operationLogData.StatusCode = trans.Ptr(statusCode)
				operationLogData.Reason = trans.Ptr(reason)
				operationLogData.Success = trans.Ptr(success)
			}
			if loginLogData != nil {
				loginLogData.StatusCode = trans.Ptr(statusCode)
				loginLogData.Reason = trans.Ptr(reason)
				loginLogData.Success = trans.Ptr(success)
			}

			// 异步落库：投递到队列即返回，不阻塞响应关键路径
			if (operationLogData != nil && writer.writeOperationLogFunc != nil) ||
				(loginLogData != nil && writer.writeLoginLogFunc != nil) {
				writer.enqueue(logWriteTask{operationLog: operationLogData, loginLog: loginLogData})
			}

			return
		}
	}
}

// fillLoginLog 填充登录日志
func fillLoginLog(htr *http.Transport) *adminV1.AdminLoginLog {
	if htr.Operation() != adminV1.OperationAuthenticationServiceLogin {
		return nil
	}

	loginLogData := &adminV1.AdminLoginLog{}

	clientIp := GetClientRealIP(htr.Request())

	loginLogData.LoginIp = trans.Ptr(clientIp)
	loginLogData.LoginTime = timeutil.TimeToTimestamppb(trans.Ptr(time.Now()))

	loginLogData.Location = trans.Ptr(clientIpToLocation(clientIp))

	if username, err := BindLoginRequest(htr.Request()); err == nil {
		loginLogData.Username = trans.Ptr(username)
	}

	// 获取客户端ID
	loginLogData.ClientId = trans.Ptr(getClientID(htr.Request(), nil))

	// 用户代理信息
	strUserAgent := htr.RequestHeader().Get(HeaderKeyUserAgent)
	ua := useragent.Parse(strUserAgent)

	var deviceName string
	if ua.Device != "" {
		deviceName = ua.Device
	} else {
		if ua.Desktop {
			deviceName = "PC"
		}
	}

	loginLogData.UserAgent = trans.Ptr(ua.String)
	loginLogData.BrowserVersion = trans.Ptr(ua.Version)
	loginLogData.BrowserName = trans.Ptr(ua.Name)
	loginLogData.OsName = trans.Ptr(ua.OS)
	loginLogData.OsVersion = trans.Ptr(ua.OSVersion)
	loginLogData.ClientName = trans.Ptr(deviceName)

	return loginLogData
}

// fillOperationLog 填充操作日志
func fillOperationLog(htr *http.Transport) *adminV1.AdminOperationLog {
	if htr.Operation() == adminV1.OperationAuthenticationServiceLogin {
		return nil
	}

	operationLogData := &adminV1.AdminOperationLog{}

	clientIp := GetClientRealIP(htr.Request())
	referer, _ := url.QueryUnescape(htr.RequestHeader().Get(HeaderKeyReferer))
	requestUri, _ := url.QueryUnescape(htr.Request().RequestURI)
	bodyBytes, _ := io.ReadAll(htr.Request().Body)
	// 读后恢复 body，保证下游仍可读取
	htr.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))

	operationLogData.Method = trans.Ptr(htr.Request().Method)
	operationLogData.Operation = trans.Ptr(htr.Operation())
	operationLogData.Path = trans.Ptr(htr.PathTemplate())
	operationLogData.Referer = trans.Ptr(referer)
	operationLogData.ClientIp = trans.Ptr(clientIp)
	operationLogData.RequestId = trans.Ptr(getRequestId(htr.Request()))
	operationLogData.RequestUri = trans.Ptr(requestUri)
	operationLogData.RequestBody = trans.Ptr(string(bodyBytes))
	operationLogData.Location = trans.Ptr(clientIpToLocation(clientIp))

	ut := extractAuthToken(htr)
	if ut != nil {
		operationLogData.UserId = trans.Ptr(ut.UserId)
		operationLogData.Username = ut.Username
	}

	// 获取客户端ID
	operationLogData.ClientId = trans.Ptr(getClientID(htr.Request(), ut))

	// 用户代理信息
	strUserAgent := htr.RequestHeader().Get(HeaderKeyUserAgent)
	ua := useragent.Parse(strUserAgent)

	var deviceName string
	if ua.Device != "" {
		deviceName = ua.Device
	} else {
		if ua.Desktop {
			deviceName = "PC"
		}
	}

	operationLogData.UserAgent = trans.Ptr(ua.String)
	operationLogData.BrowserVersion = trans.Ptr(ua.Version)
	operationLogData.BrowserName = trans.Ptr(ua.Name)
	operationLogData.OsName = trans.Ptr(ua.OS)
	operationLogData.OsVersion = trans.Ptr(ua.OSVersion)
	operationLogData.ClientName = trans.Ptr(deviceName)

	return operationLogData
}
