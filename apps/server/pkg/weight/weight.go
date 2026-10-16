// Package weight 提供基于实例权重的 kratos 服务选择器，
// 配合服务注册实例元数据 weight 使用（《00-总体方案/04-服务注册与发现规范》§6）。
// 移植自参考工程 utils/weight，行为保持一致：从候选节点中选择权重最高者。
package weight

import (
	"context"
	"sync"

	"github.com/go-kratos/kratos/v2/selector"
	"github.com/go-kratos/kratos/v2/selector/node/direct"
)

// Name 是权重负载均衡器名称。
const Name = "weight"

var _ selector.Balancer = (*Balancer)(nil)

// Balancer 权重选择均衡器。
type Balancer struct {
	mu            sync.Mutex
	currentWeight map[string]float64
}

// New 创建权重选择器。
func New() selector.Selector {
	return NewBuilder().Build()
}

// Pick 从候选节点中挑选权重最高的节点。
// nginx wrr load balancing algorithm: http://blog.csdn.net/zhangskd/article/details/50194069
func (p *Balancer) Pick(_ context.Context, nodes []selector.WeightedNode) (selector.WeightedNode, selector.DoneFunc, error) {
	if len(nodes) == 0 {
		return nil, nil, selector.ErrNoAvailable
	}
	var selected selector.WeightedNode
	var selectWeight float64

	p.mu.Lock()
	for _, node := range nodes {
		if selected == nil || selectWeight < node.Weight() {
			selectWeight = node.Weight()
			selected = node
		}
	}
	p.mu.Unlock()

	return selected, selected.Pick(), nil
}

// NewBuilder 返回权重选择器构造器（全局选择器：selector.SetGlobalSelector(weight.NewBuilder())）。
func NewBuilder() selector.Builder {
	return &selector.DefaultBuilder{
		Balancer: &Builder{},
		Node:     &direct.Builder{},
	}
}

// Builder 是权重均衡器构造器。
type Builder struct{}

// Build 创建 Balancer。
func (b *Builder) Build() selector.Balancer {
	return &Balancer{currentWeight: make(map[string]float64)}
}
