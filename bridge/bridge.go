package bridge

import (
	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/dom"
)

type Bridge interface {
	ApplyMutations(muts []core.Mutation)
	RequestAnimationFrame(fn func())
	StartScheduler()
}

type NoopBridge struct {
	dom *dom.NodeRegistry
}

func NewNoopBridge() *NoopBridge {
	return &NoopBridge{dom: dom.NewNodeRegistry()}
}

func (b *NoopBridge) ApplyMutations(muts []core.Mutation) {}

func (b *NoopBridge) RequestAnimationFrame(fn func()) {
	fn()
}

func (b *NoopBridge) StartScheduler() {}
