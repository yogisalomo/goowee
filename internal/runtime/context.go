package runtime

import (
	"sync"

	"github.com/yogisalomo/goowee/core"
)

type Env int

const (
	EnvClient Env = iota
	EnvServer
)

type RenderContext struct {
	Env   Env
	stack []*core.ComponentFrame
}

func NewRenderContext(env Env) *RenderContext {
	return &RenderContext{Env: env}
}

func (c *RenderContext) push() *core.ComponentFrame {
	parent := c.currentFrame()
	path := "/"
	if parent != nil {
		path = parent.Path + "/" + itoa(len(parent.Children))
	}
	node := &core.ComponentFrame{Path: path, Parent: parent}
	if parent != nil {
		parent.Children = append(parent.Children, node)
	}
	c.stack = append(c.stack, node)
	return node
}

func (c *RenderContext) pop() {
	if len(c.stack) > 0 {
		c.stack = c.stack[:len(c.stack)-1]
	}
}

func (c *RenderContext) currentFrame() *core.ComponentFrame {
	if len(c.stack) == 0 {
		return nil
	}
	return c.stack[len(c.stack)-1]
}

var (
	ambientMu sync.Mutex
	ambient   *RenderContext
	// defaultContext backs the ambient frame functions when no explicit
	// context is installed — i.e. the single-threaded client (WASM) and
	// direct test usage.
	defaultContext = NewRenderContext(EnvClient)
)

func UseContext(ctx *RenderContext, fn func()) {
	ambientMu.Lock()
	defer ambientMu.Unlock()
	prev := ambient
	ambient = ctx
	defer func() { ambient = prev }()
	fn()
}

func activeContext() *RenderContext {
	if ambient != nil {
		return ambient
	}
	return defaultContext
}

func PushComponent() *core.ComponentFrame { return activeContext().push() }
func PopComponent()                       { activeContext().pop() }

func SaveFrameStack() int { return len(activeContext().stack) }
func RestoreFrameStack(depth int) {
	c := activeContext()
	if depth >= 0 && depth <= len(c.stack) {
		c.stack = c.stack[:depth]
	}
}

func CurrentComponent() *core.ComponentFrame {
	return activeContext().currentFrame()
}

func CurrentEnv() Env { return activeContext().Env }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func init() {
	core.RegisterDisposer = func(fn func()) {
		if f := CurrentComponent(); f != nil {
			f.Disposers = append(f.Disposers, fn)
		}
	}
}
