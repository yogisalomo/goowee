package h

import "github.com/yogisalomo/goowee/core"

type HandlerOption func(*core.HandlerOptions)

// PreventDefault always cancels the event's default action. To decide per
// event, call e.PreventDefault() inside the handler instead.
func PreventDefault() HandlerOption {
	return func(o *core.HandlerOptions) { o.PreventDefault = true }
}

// StopPropagation always stops the event from reaching ancestors' handlers
// (handlers run innermost first). To decide per event, call
// e.StopPropagation() inside the handler instead.
func StopPropagation() HandlerOption {
	return func(o *core.HandlerOptions) { o.StopPropagation = true }
}

func On(event string, fn func(core.EventData), opts ...HandlerOption) core.Item {
	h := core.Handler{Event: event, Fn: fn}
	for _, o := range opts {
		o(&h.Options)
	}
	return handlerItem{h}
}

func OnClick(fn func(), opts ...HandlerOption) core.Item {
	return On("click", func(core.EventData) { fn() }, opts...)
}

func OnClickE(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("click", fn, opts...)
}

func OnDblClick(fn func(), opts ...HandlerOption) core.Item {
	return On("dblclick", func(core.EventData) { fn() }, opts...)
}

func OnDblClickE(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("dblclick", fn, opts...)
}

func OnInput(fn func(value string), opts ...HandlerOption) core.Item {
	return On("input", func(e core.EventData) { fn(e.Value()) }, opts...)
}

func OnInputE(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("input", fn, opts...)
}

func OnChange(fn func(value string), opts ...HandlerOption) core.Item {
	return On("change", func(e core.EventData) { fn(e.Value()) }, opts...)
}

func OnChangeE(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("change", fn, opts...)
}

func OnSubmit(fn func(values map[string]string), opts ...HandlerOption) core.Item {
	return On("submit",
		func(e core.EventData) { fn(e.FormValues()) },
		append(opts, PreventDefault())...)
}

func OnKeyDown(fn func(key string), opts ...HandlerOption) core.Item {
	return On("keydown", func(e core.EventData) { fn(e.Key()) }, opts...)
}

func OnKeyDownE(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("keydown", fn, opts...)
}

func OnKeyUp(fn func(key string), opts ...HandlerOption) core.Item {
	return On("keyup", func(e core.EventData) { fn(e.Key()) }, opts...)
}

func OnKeyUpE(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("keyup", fn, opts...)
}

func OnFocus(fn func(), opts ...HandlerOption) core.Item {
	return On("focus", func(core.EventData) { fn() }, opts...)
}

func OnBlur(fn func(), opts ...HandlerOption) core.Item {
	return On("blur", func(core.EventData) { fn() }, opts...)
}

func OnScroll(fn func(scrollTop float64), opts ...HandlerOption) core.Item {
	return On("scroll", func(e core.EventData) { fn(e.ScrollTop()) }, opts...)
}

func OnScrollE(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("scroll", fn, opts...)
}

func OnReset(fn func(), opts ...HandlerOption) core.Item {
	return On("reset", func(core.EventData) { fn() }, opts...)
}

func OnInvalid(fn func(), opts ...HandlerOption) core.Item {
	return On("invalid", func(core.EventData) { fn() }, opts...)
}

func OnPaste(fn func(value string), opts ...HandlerOption) core.Item {
	return On("paste", func(e core.EventData) { fn(e.Value()) }, opts...)
}

func OnCut(fn func(value string), opts ...HandlerOption) core.Item {
	return On("cut", func(e core.EventData) { fn(e.Value()) }, opts...)
}

func OnCopy(fn func(value string), opts ...HandlerOption) core.Item {
	return On("copy", func(e core.EventData) { fn(e.Value()) }, opts...)
}

func OnFocusIn(fn func(), opts ...HandlerOption) core.Item {
	return On("focusin", func(core.EventData) { fn() }, opts...)
}

func OnFocusOut(fn func(), opts ...HandlerOption) core.Item {
	return On("focusout", func(core.EventData) { fn() }, opts...)
}

// SelectOnFocus returns a handler option that selects all text when the
// element receives focus — a common UX pattern for input and textarea fields.
func SelectOnFocus() HandlerOption {
	return func(o *core.HandlerOptions) { o.SelectOnFocus = true }
}

// OnMouseEnter / OnMouseLeave fire when the pointer enters or leaves the
// element itself (they don't bubble — moving between its children doesn't
// fire them).
func OnMouseEnter(fn func(), opts ...HandlerOption) core.Item {
	return On("mouseenter", func(core.EventData) { fn() }, opts...)
}

func OnMouseLeave(fn func(), opts ...HandlerOption) core.Item {
	return On("mouseleave", func(core.EventData) { fn() }, opts...)
}

// OnPointerDown / OnPointerMove / OnPointerUp receive position, buttons,
// pointerId and pointerType (e.ClientX(), e.PointerType(), …). Pointer moves
// are coalesced to one per frame per element.
func OnPointerDown(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("pointerdown", fn, opts...)
}

func OnPointerMove(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("pointermove", fn, opts...)
}

func OnPointerUp(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("pointerup", fn, opts...)
}

// OnWheel receives e.DeltaX()/DeltaY()/DeltaMode() and the pointer position.
func OnWheel(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("wheel", fn, opts...)
}

// OnContextMenu fires on a right-click / long-press; call e.PreventDefault()
// to replace the browser's menu with your own.
func OnContextMenu(fn func(core.EventData), opts ...HandlerOption) core.Item {
	return On("contextmenu", fn, opts...)
}

// OnLoad / OnError fire when an <img>, <script>, <iframe>, or media element
// finishes loading or fails to.
func OnLoad(fn func(), opts ...HandlerOption) core.Item {
	return On("load", func(core.EventData) { fn() }, opts...)
}

func OnError(fn func(), opts ...HandlerOption) core.Item {
	return On("error", func(core.EventData) { fn() }, opts...)
}
