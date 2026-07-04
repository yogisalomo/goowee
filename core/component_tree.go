package core

import "sync"

type ComponentFrame struct {
	Path        string
	Parent      *ComponentFrame
	Children    []*ComponentFrame
	RootNodeIDs []int
	Hooks       []any
	Disposers   []func()
}

// Env distinguishes a client render (WASM, effects run) from a server
// render (SSR, effects must not run — no lifecycle, no goroutines).
type Env int

const (
	EnvClient Env = iota
	EnvServer
)

// RenderContext owns the component-frame stack for a single render pass.
// Making it a value (rather than a package global) is what lets concurrent
// server renders keep separate stacks instead of corrupting each other.
type RenderContext struct {
	Env   Env
	stack []*ComponentFrame
}

func NewRenderContext(env Env) *RenderContext {
	return &RenderContext{Env: env}
}

func (c *RenderContext) push() *ComponentFrame {
	parent := c.currentFrame()
	path := "/"
	if parent != nil {
		path = parent.Path + "/" + itoa(len(parent.Children))
	}
	node := &ComponentFrame{Path: path, Parent: parent}
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

func (c *RenderContext) currentFrame() *ComponentFrame {
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

// UseContext installs ctx as the active render context for the duration of
// fn, then restores the previous context. Calls are serialized, so
// concurrent server renders never share a frame stack, and a fresh ctx per
// render means a panic mid-render can't corrupt the next render's stack.
//
// The client render path does not use this (it runs on the default context,
// single-threaded); it exists for the SSR renderer, whose handler goroutines
// would otherwise race on the shared stack.
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

func PushComponent() *ComponentFrame { return activeContext().push() }
func PopComponent()                  { activeContext().pop() }
func CurrentComponent() *ComponentFrame {
	return activeContext().currentFrame()
}

// CurrentEnv reports whether the active render is client- or server-side.
func CurrentEnv() Env { return activeContext().Env }

func RegisterDisposer(fn func()) {
	if f := CurrentComponent(); f != nil {
		f.Disposers = append(f.Disposers, fn)
	}
}

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
