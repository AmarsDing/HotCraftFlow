import axios, { type AxiosInstance, type AxiosRequestConfig, type AxiosResponse, type InternalAxiosRequestConfig, type Method } from 'axios';
import { Notify } from 'quasar';
import { useLicenseStore } from '../stores/license';

// 允许按请求关闭拦截器的全局错误 toast：调用方在 catch 中自行给出更精确提示的场景
//（如 409 重复标记已发送），避免拦截器 negative + 页面 warning 双 toast。
declare module 'axios' {
  interface AxiosRequestConfig {
    skipErrorNotify?: boolean;
    /** 401 刷新后已重试过一次：上游确定性 401（如 JWT 密钥漂移）时防止 refresh→retry 死循环 */
    _retriedAfterRefresh?: boolean;
    /** 不登记路由切换取消（logout 等请求需在跳转登录页后仍在后台完成，不被 abortRoutePendingRequests 中止） */
    skipAbortRegister?: boolean;
  }
}

// ===== 路由切换取消在途请求 =====
// 页面跳走后，上一页未完成的请求仍占用后端连接与带宽，会拖慢新页面首屏
//（实测仪表盘 windfield 大请求把综合运维中心接口拖慢 3-5 倍）。
// 经 instance 的请求默认登记 AbortController，路由 path 变化时统一取消；
// 已在 config 上自带 signal 的请求保持调用方语义，不接管。
const pendingControllers = new Set<AbortController>();
const pendingByConfig = new WeakMap<object, AbortController>();

function settlePending(config?: object) {
  if (!config) return;
  const ctl = pendingByConfig.get(config);
  if (ctl) {
    pendingControllers.delete(ctl);
    pendingByConfig.delete(config);
  }
}

// 路由 path 变化时调用（router afterEach）：取消上一页面所有在途请求
export function abortRoutePendingRequests() {
  for (const ctl of pendingControllers) ctl.abort();
  pendingControllers.clear();
}

// baseURL 留空：请求统一使用 /api/v1/monitor/... 完整路径，
// 开发环境经 quasar devServer proxy 转发到 44 测试环境网关 192.168.4.44:8000，由网关按前缀分流到各上游服务
const instance: AxiosInstance = axios.create({
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
});

instance.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('framework_access_token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    const lang = localStorage.getItem('locale') || 'zh-CN';
    config.headers['Accept-Language'] = lang;
    if (!config.signal && !config.skipAbortRegister) {
      const ctl = new AbortController();
      config.signal = ctl.signal;
      pendingControllers.add(ctl);
      pendingByConfig.set(config, ctl);
    }
    return config;
  },
  (error) => Promise.reject(error),
);

// ===== 登出进行中标记 =====
// 登出期间（含跳转登录页后短暂窗口）页面残留的轮询/在途请求会 401，
// 此时禁止进入 refresh→retry→clearSessionAndRedirect 全量刷新链路（与 router.push('/login') 竞态，
// 表现为"退出慢+报错误"）；登录成功后由 auth store 复位。
let loggingOut = false;
export function setLoggingOut(v: boolean) {
  loggingOut = v;
}

// ===== 401 token 刷新重试机制 =====
// 多个请求并发 401 时，仅刷新一次，其余请求排队等待新 token 后重试
let isRefreshing = false;
let pendingQueue: Array<{ resolve: (token: string) => void; reject: (err: unknown) => void }> = [];

function flushQueue(token: string | null, error: unknown = null) {
  pendingQueue.forEach((p) => {
    if (token) p.resolve(token);
    else p.reject(error);
  });
  pendingQueue = [];
}

async function refreshAccessToken(): Promise<string | null> {
  const refreshToken = localStorage.getItem('framework_refresh_token');
  if (!refreshToken) return null;
  try {
    // 直接用 instance 调用（绕过 authService 避免循环依赖）；响应拦截器会返回 data
    const data = await instance.post('/api/v1/monitor/auth/refresh', {
      grant_type: 'refresh_token',
      refresh_token: refreshToken,
    });
    const resp = data as unknown as { access_token?: string; refresh_token?: string };
    if (resp?.access_token) {
      localStorage.setItem('framework_access_token', resp.access_token);
      if (resp.refresh_token) {
        localStorage.setItem('framework_refresh_token', resp.refresh_token);
      }
      return resp.access_token;
    }
    return null;
  } catch {
    return null;
  }
}

// 附件下载（responseType=blob）失败时，错误体也是 Blob，直接读 .message 会得到空，
// 页面只能看到 "Request failed"，导出/下载表现为失效。
async function readErrorMessage(error: { response?: AxiosResponse; message?: string }): Promise<string> {
  const data = error.response?.data;
  if (data instanceof Blob) {
    try {
      const text = await data.text();
      const parsed = JSON.parse(text) as { message?: string; error?: string };
      if (parsed.message || parsed.error) return parsed.message || parsed.error || 'Request failed';
    } catch {
      // 非 JSON 错误体，回退 axios 原文
    }
  } else if (data && typeof data === 'object') {
    const body = data as { message?: string; error?: string };
    if (body.message || body.error) return body.message || body.error || 'Request failed';
  }
  return error.message || 'Request failed';
}

function clearSessionAndRedirect() {
  localStorage.removeItem('framework_access_token');
  localStorage.removeItem('framework_refresh_token');
  localStorage.removeItem('framework_session');
  const cur = window.location.pathname;
  if (!cur.startsWith('/login')) {
    const redirect = encodeURIComponent(cur + window.location.search);
    window.location.href = `/login?redirect=${redirect}`;
  }
}

// ===== 许可证受限模式（§2.6.8） =====
// 网关在每个响应上携带 X-License-Status 头；restricted 时写入全局 store，
// MainLayout 据以渲染全站常驻横幅。仅在头存在时更新，缺失时保持原状态
// （直连上游/未经网关的响应不带该头，不应误清受限标记）。
function syncLicenseStatus(response?: AxiosResponse) {
  const value = response?.headers?.['x-license-status'];
  if (typeof value !== 'string' || value === '') return;
  try {
    useLicenseStore().setRestricted(value === 'restricted');
  } catch {
    // pinia 未激活（如登录前请求）时忽略，横幅随后续请求同步
  }
}

instance.interceptors.response.use(
  (response) => {
    settlePending(response.config);
    syncLicenseStatus(response);
    return response.data;
  },
  async (error) => {
    settlePending(error.config);
    // 路由切换主动取消：静默拒绝，不弹错误 toast、不进 401 刷新流程
    if (axios.isCancel(error) || error.code === 'ERR_CANCELED') {
      return Promise.reject(error);
    }
    syncLicenseStatus(error.response as AxiosResponse | undefined);
    const status = error.response?.status;
    const message = await readErrorMessage(error);

    if (status === 401) {
      // 登出进行中：静默拒绝，不刷新、不跳转、不弹 toast（本地会话已主动清理，401 属预期）
      if (loggingOut) {
        return Promise.reject(error);
      }
      const reqUrl = (error.config as InternalAxiosRequestConfig | undefined)?.url ?? '';
      // 业务级 401（非 token 失效）：二次鉴权操作密码错误等，直接走通知+reject，
      // 禁止进入 refresh 流程（否则 retry 仍 401 会形成 refresh→retry 死循环，调用方悬挂）
      const reason = error.response?.data?.reason;
      if (reason === 'OPERATION_PASSWORD_INVALID') {
        if (!error.config?.skipErrorNotify) {
          Notify.create({ type: 'negative', message, position: 'top', timeout: 3000 });
        }
        return Promise.reject(error);
      }
      // refresh 请求本身 401，直接登出（避免死循环：refresh 期间 isRefreshing=true，
      // 若 refresh 请求也进入排队分支将永久等待自己完成，造成应用卡死在空白页）
      if (reqUrl.includes('/auth/refresh')) {
        flushQueue(null, error);
        clearSessionAndRedirect();
        return Promise.reject(error);
      }
      // 已刷新重试过仍 401：上游确定性拒绝（如该服务 JWT 密钥与签发方漂移），
      // 再刷只会 refresh→retry 死循环打满网关限流（2026-09 monitorquery 密钥漂移实证），直接失败。
      if (error.config?._retriedAfterRefresh) {
        if (!error.config.skipErrorNotify) {
          Notify.create({ type: 'negative', message, position: 'top', timeout: 3000 });
        }
        return Promise.reject(error);
      }
      // 已在刷新中：排队等待
      if (isRefreshing) {
        return new Promise((resolve, reject) => {
          pendingQueue.push({
            resolve: (token: string) => {
              if (error.config) {
                error.config.headers.Authorization = `Bearer ${token}`;
                error.config._retriedAfterRefresh = true;
                resolve(instance(error.config));
              } else {
                reject(error);
              }
            },
            reject,
          });
        });
      }
      // 首次 401：尝试刷新
      isRefreshing = true;
      const newToken = await refreshAccessToken();
      isRefreshing = false;
      if (newToken) {
        flushQueue(newToken);
        if (error.config) {
          error.config.headers.Authorization = `Bearer ${newToken}`;
          error.config._retriedAfterRefresh = true;
          return instance(error.config);
        }
      }
      // 刷新失败，清空队列并登出
      flushQueue(null, error);
      clearSessionAndRedirect();
      return Promise.reject(error);
    }

    if (!error.config?.skipErrorNotify) {
      Notify.create({ type: 'negative', message, position: 'top', timeout: 3000 });
    }
    return Promise.reject(error);
  },
);

export interface PagingRequest {
  page?: number;
  pageSize?: number;
  noPaging?: boolean;
  query?: string;
  orQuery?: string;
  fieldMask?: string;
  orderBy?: string;
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}

export interface UpdateRequest<T> {
  data: T;
  updateMask?: string;
  allowMissing?: boolean;
}

export const request = {
  get<T = unknown>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return instance.get(url, config);
  },
  post<T = unknown>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return instance.post(url, data, config);
  },
  put<T = unknown>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return instance.put(url, data, config);
  },
  patch<T = unknown>(url: string, data?: unknown, config?: AxiosRequestConfig): Promise<T> {
    return instance.patch(url, data, config);
  },
  delete<T = unknown>(url: string, config?: AxiosRequestConfig): Promise<T> {
    return instance.delete(url, config);
  },
};

export function buildQueryParams(
  formValues: Record<string, unknown> | undefined,
  opts: {
    page?: number;
    pageSize?: number;
    noPaging?: boolean;
    orderBy?: string;
    fieldMask?: string;
  } = {},
): Record<string, string> {
  const params: Record<string, string> = {};
  if (opts.noPaging) {
    params.noPaging = 'true';
  } else {
    params.page = String(opts.page ?? 1);
    params.pageSize = String(opts.pageSize ?? 10);
  }
  if (formValues && Object.keys(formValues).length > 0) {
    const cleaned: Record<string, string> = {};
    for (const [k, v] of Object.entries(formValues)) {
      if (v !== undefined && v !== null && v !== '') cleaned[k] = String(v);
    }
    if (Object.keys(cleaned).length > 0) {
      params.query = JSON.stringify(cleaned);
    }
  }
  if (opts.orderBy) params.orderBy = opts.orderBy;
  if (opts.fieldMask) params.fieldMask = opts.fieldMask;
  return params;
}

export function buildUpdateMask(values: Record<string, unknown>): string {
  // 仅过滤 undefined（未触碰字段）；保留 null —— null 是"显式置空"语义，
  // 后端 data 层 NilValuePaths 会据 updateMask 中的 null 路径生成 SetNull（可选字段清空依赖此链路）。
  return Object.keys(values)
    .filter((k) => values[k] !== undefined)
    .join(',');
}

// ===== 生成客户端的 requestHandler =====
// protoc-gen-typescript-http 生成的客户端通过 handler 回调发起请求，
// handler 接收 {path, method, body} 并返回 Promise<unknown>（响应数据）。
// 响应拦截器已自动解包 response.data，因此直接返回 axios 结果即可。
export interface GeneratedRequest {
  path: string;
  method: string;
  body: string | null;
}
export interface RequestMeta {
  service: string;
  method: string;
}
export type RequestHandler = (request: GeneratedRequest, meta: RequestMeta) => Promise<unknown>;

// 自行处理错误提示的生成客户端接口（"service.method"）：拦截器不弹全局错误 toast，
// 由页面在 catch 中给出精确提示（如 409 重复标记已发送 → warning「记录已是已发送状态」；
// MetarService.GetLatestReport 站点暂无报文 404 属预期空态，picker 气象卡静默回退不打扰）。
const SELF_HANDLED_ERROR_APIS = new Set([
  'NoticeRecordService.MarkRecordSent',
  'MetarService.GetLatestReport',
  // RCA 触发门控统计（13-AI智能运维 FR-2）：旧版 aiops 容器无 gate 路由 404 属预期，
  // 洞察看板卡片静默降级为「门控服务未就绪」提示，不弹全局错误 toast
  'RcaGateService.GetGateStats',
  // 雷达时间轴监控页统一在 catch 中提示「时间轴数据加载失败」，避免拦截器 negative 与页面提示双 toast
  'MonitorRadarTimelineService.GetTimelineHeatmap',
  'MonitorRadarTimelineService.GetSiteFreshnessBoard',
  'MonitorRadarTimelineService.ListDelayedFiles',
  // 登出为后台撤销（fire-and-forget）：失败静默，页面已跳转登录页，不弹事后错误 toast
  'AuthenticationService.Logout',
]);

// 长耗时实时探测接口的超时分档（键 = service.method）：后端执行本身可达数十秒
// （路由追踪 30 跳 × 逐跳超时、MTU 二分、SNMP walk 重试、SSL 批量握手），网关拥塞时
// 还有排队余量；一刀切 30s 会把正常长操作误杀（2026-08-21 在线诊断页实测复现）。
const API_TIMEOUT_OVERRIDES = new Map<string, number>([
  ['DiagnoseService.Ping', 60000], // 网段单批 64 目标 ×2 包的服务端最坏耗时 + 排队余量
  ['DiagnoseService.Traceroute', 180000], // 30 跳 × 逐跳超时，后端最坏分钟级
  ['DiagnoseService.HealthCheck', 90000], // 串行 网关→DNS→端口→SSL
  ['DiagnoseService.SslCheck', 60000], // 多目标串行 TLS 握手
  ['DiagnoseService.MtuDetect', 120000], // 二分探测，页内提示"全程最长约 30 秒"
  ['DiagnoseService.NeighborProbe', 90000], // SNMP walk 超时重试
  // F17 测速：打流 ≤30s + server 宽限 5s + 拨号/收尾余量（2026-08-25 审查：默认 30s 会误杀长测试）
  ['DiagnoseService.ThroughputTest', 60000],
  // F18 会话压测：极限梯度预算 5×(5+10+2)+10=95s（UI 侧另有预算校验拦截超网关组合）
  ['DiagnoseService.SessionStressTest', 150000],
  // 采集失败/Trap 日志手动清空 + 立即清理：百万行级 TRUNCATE/分批 DELETE，与后端 30min 对齐
  ['FailureService.ClearFailures', 1800000],
  ['FailureService.RunRetentionNow', 1800000],
  ['TrapService.ClearTrapEvents', 1800000],
  // AI 智能运维（aiops）：LLM 类接口天然分钟级——aranea 侧 graph 运行 + 模型流式生成实测 30~60s，
  // 后端 TestAgent 轮询预算 55s、RCA 分析 rca_timeout_seconds=300。默认 30s 会让 axios 先 abort，
  // 前端只拿到通用失败提示（2026-09-22 44 环境实测：直连 aiops 200/34.1s，经网关 30s 处 502）。
  // 与网关路由 timeout_ms 对齐（gateway seed「智能运维」/api/v1/monitor/aiops = 300000）。
  ['AiAgentService.TestAgent', 300000],
  ['AiRcaService.AnalyzeRcaRecord', 300000],
  ['AiAutoAnalysisService.TriggerAutoAnalysis', 300000],
]);

// 服务级超时：与网关路由 timeout_ms 对齐，避免 axios 30s 先 abort → 网关记 context canceled。
function serviceTimeoutMs(service: string, method: string): number | undefined {
  if (service === 'AnalysisService') {
    if (method.toLowerCase().includes('backup')) return 120000;
    return 60000;
  }
  if (service === 'MonitorQueryService' || service === 'QueryService') return 60000;
  return undefined;
}

export const requestHandler: RequestHandler = (request, meta) => {
  const url = '/' + request.path.replace(/^\//, '');
  const headers: Record<string, string> = {};
  if (request.body !== null) {
    headers['Content-Type'] = 'application/json';
  }
  const key = `${meta.service}.${meta.method}`;
  return instance.request({
    url,
    method: request.method as Method,
    data: request.body ?? undefined,
    headers,
    timeout: API_TIMEOUT_OVERRIDES.get(key) ?? serviceTimeoutMs(meta.service, meta.method),
    skipErrorNotify: SELF_HANDLED_ERROR_APIS.has(key),
    // logout 在后台完成服务端撤销：跳转登录页触发的路由取消不得中止它
    skipAbortRegister: key === 'AuthenticationService.Logout',
  });
};
