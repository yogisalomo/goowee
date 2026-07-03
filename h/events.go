package h

import "goowee/core"

type HandlerOption func(*core.HandlerOptions)

func PreventDefault() HandlerOption {
	return func(o *core.HandlerOptions) { o.PreventDefault = true }
}

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
