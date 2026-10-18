package engines

import (
	"context"
	"sync"

	"framework/app/media/internal/hostcaps"
)

// View 引擎清单项（service 层直接投影到 pb EngineView）。
type View struct {
	ID            string
	Kind          Kind
	DisplayName   string
	Caps          Caps
	Available     bool
	Reason        string
	Routing       Routing
	RoutingNotice string
	Selected      bool
}

// lazyProvider 惰性注册的引擎工厂（重依赖引擎在首次访问时才装配）。
type lazyProvider func() Engine

// Registry 引擎注册表：急切 map + 惰性 provider（对齐双注册表思想），
// 持当前激活引擎与原子的单引擎驻留纪律钩子。
type Registry struct {
	mu     sync.RWMutex
	eager  map[string]Engine
	lazy   map[string]lazyProvider
	active string

	host      hostcaps.Caps
	onEvict   func(id string) // 单引擎驻留：换引擎前回调（如 sidecar 收割）
}

// NewRegistry 建立注册表；host 在启动时探测一次并贯穿（重探测走新的一轮 List）。
func NewRegistry(host hostcaps.Caps, onEvict func(id string)) *Registry {
	return &Registry{
		eager:   make(map[string]Engine),
		lazy:    make(map[string]lazyProvider),
		host:    host,
		onEvict: onEvict,
	}
}

// Register 急切注册（启动即装配，如 echo 引擎）。
func (r *Registry) Register(e Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.eager[e.ID()] = e
}

// RegisterLazy 惰性注册（首次 List/resolve 时才装配；装配失败记录为不可用）。
func (r *Registry) RegisterLazy(id string, p lazyProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lazy[id] = p
}

// resolve 取引擎实例（惰性条目即时装配；装配成功转急切缓存）。
func (r *Registry) resolve(id string) (Engine, error) {
	r.mu.RLock()
	if e, ok := r.eager[id]; ok {
		r.mu.RUnlock()
		return e, nil
	}
	p, ok := r.lazy[id]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrEngineNotFound
	}
	e := p()
	if e == nil {
		return nil, ErrEngineUnavailable
	}
	r.mu.Lock()
	r.eager[id] = e
	delete(r.lazy, id)
	r.mu.Unlock()
	return e, nil
}

// IDs 全部注册 id（急切 + 惰性）。
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.eager)+len(r.lazy))
	for id := range r.eager {
		out = append(out, id)
	}
	for id := range r.lazy {
		out = append(out, id)
	}
	return out
}

// List 清单视图：逐引擎跑 Available（异常收敛为不可用的稳定 reason）、
// 计算路由与激活标记。
func (r *Registry) List(ctx context.Context) []View {
	ids := r.IDs()
	active := r.ActiveID()
	out := make([]View, 0, len(ids))
	for _, id := range ids {
		v := View{ID: id, Selected: id == active}
		e, err := r.resolve(id)
		if err != nil {
			v.Available = false
			v.Reason = stableReason(err)
			v.Routing = RoutingUnavailable
			v.RoutingNotice = Notice(RoutingUnavailable, Caps{}, r.host)
			out = append(out, v)
			continue
		}
		v.Kind = e.Kind()
		v.DisplayName = e.DisplayName()
		v.Caps = e.Caps()
		rt := Resolve(v.Caps, r.host)
		v.Routing = rt
		ok, reason := e.Available(ctx)
		v.Available = ok && rt != RoutingUnavailable
		switch {
		case !ok:
			v.Reason = reason
		case rt == RoutingUnavailable:
			v.Available = false
			v.Reason = Notice(rt, v.Caps, r.host)
		}
		v.RoutingNotice = Notice(rt, v.Caps, r.host)
		out = append(out, v)
	}
	return out
}

// Select 激活引擎：拒绝 unavailable（不静默降级）；
// 单引擎驻留纪律——换引擎前回调 evict 旧实例。
func (r *Registry) Select(ctx context.Context, id string) (View, error) {
	e, err := r.resolve(id)
	if err != nil {
		return View{}, err
	}
	caps := e.Caps()
	rt := Resolve(caps, r.host)
	if err := GateError(rt); err != nil {
		return View{}, err
	}
	ok, reason := e.Available(ctx)
	if !ok {
		return View{ID: id, Available: false, Reason: reason, Routing: rt}, ErrEngineUnavailable
	}

	r.mu.Lock()
	prev := r.active
	if prev != id && prev != "" && r.onEvict != nil {
		r.onEvict(prev)
	}
	r.active = id
	r.mu.Unlock()

	return View{
		ID: id, Kind: e.Kind(), DisplayName: e.DisplayName(), Caps: caps,
		Available: true, Routing: rt, RoutingNotice: Notice(rt, caps, r.host), Selected: true,
	}, nil
}

// EngineFor 取指定引擎实例（惰性条目即时装配）；不改全局激活态。
func (r *Registry) EngineFor(id string) (Engine, error) {
	return r.resolve(id)
}

// ActiveID 当前激活引擎 id。
func (r *Registry) ActiveID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active
}

// Active 当前激活引擎；未选择时回退 defaultID（仍过路由硬门禁）。
func (r *Registry) Active(defaultID string) (Engine, Routing, error) {
	id := r.ActiveID()
	if id == "" {
		id = defaultID
	}
	e, err := r.resolve(id)
	if err != nil {
		return nil, "", err
	}
	rt := Resolve(e.Caps(), r.host)
	if err := GateError(rt); err != nil {
		return nil, rt, err
	}
	return e, rt, nil
}

// stableReason 外部错误的稳定公开映射（内部错误绝不出边界）。
func stableReason(err error) string {
	switch err {
	case ErrEngineNotFound:
		return "引擎未注册"
	case ErrEngineUnavailable, ErrEngineNoCPUFallbk:
		return "引擎在本机不可用"
	case ErrEngineLicense:
		return "模型许可证未接受"
	default:
		return "引擎装配失败"
	}
}
