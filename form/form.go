// Package form keeps a form's state in one typed value, with validation that
// the browser and the server share (#150).
//
// The value is a plain struct whose Validate method checks it. The same
// method runs in the browser, as the user types and when they submit, and on
// the server, in the handler that receives the submission:
//
//	type Signup struct {
//	    Email string
//	    Age   int
//	}
//
//	func (s Signup) Validate() form.Errors {
//	    var errs form.Errors
//	    if !strings.Contains(s.Email, "@") {
//	        errs = errs.Add("Email", "Enter an email address.")
//	    }
//	    return errs
//	}
//
// In a component:
//
//	f := form.Use(Signup{})
//	return Form(
//	    f.OnSubmit(func(ctx context.Context, v Signup) error { _, err := api.Signup.Call(ctx, v); return err }),
//	    Input(Type("email"), form.Text(f, "Email", func(s *Signup) *string { return &s.Email })),
//	    P(TextS(f.Error("Email"))),
//	    Input(Type("number"), form.Int(f, "Age", func(s *Signup) *int { return &s.Age })),
//	    Button(Type("submit"), DisabledS(f.Submitting), Text("Sign up")),
//	)
//
// And on the server (with goowee/rpc, whose *rpc.Error carries field
// errors back to the form):
//
//	rpc.Handle(mux, api.Signup, func(ctx context.Context, s Signup) (User, error) {
//	    if errs := s.Validate(); len(errs) > 0 {
//	        return User{}, rpc.Invalid("check the form", errs)
//	    }
//	    …
//	})
//
// A field's error shows once the field has been left (blur) or a submit was
// tried, and is checked again on every change after that. Field accessors
// (func(*T) *string) keep the package free of reflection.
package form

import (
	"context"
	"strconv"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/h"
	"github.com/yogisalomo/goowee/hooks"
)

// Errors maps a field's name to what is wrong with it. The empty name is the
// form as a whole (a failed submit's message, say).
type Errors map[string]string

// Add returns errs with field's message set, making the map if needed.
func (errs Errors) Add(field, message string) Errors {
	if errs == nil {
		errs = Errors{}
	}
	errs[field] = message
	return errs
}

// Validator is a form value that checks itself. Use calls Validate in the
// browser; call it on the server too.
type Validator interface {
	Validate() Errors
}

// FieldErrors is an error that carries per-field messages, as *rpc.Error
// does: a submit that fails with one shows each message on its field.
type FieldErrors interface {
	error
	FieldErrors() map[string]string
}

// Form is a form's state. Read its signals in the view; change the value
// through the field helpers (Text, Int, Bool) or Set.
type Form[T any] struct {
	Value      *core.Signal[T]
	Errors     *core.Signal[Errors]          // what Validate (or the server) found
	Touched    *core.Signal[map[string]bool] // fields the user has left
	Submitting *core.Signal[bool]            // a submit is running
	Submitted  *core.Signal[bool]            // a submit was tried: every error shows

	parse  map[string]string // fields whose text doesn't parse (Int)
	shown  map[string]*core.Signal[string]
	cancel context.CancelFunc
}

// Use creates a component's form state, starting from initial. Like the
// other hooks it belongs in a component's render function. A submit still
// running when the component unmounts is cancelled.
func Use[T any](initial T) *Form[T] {
	value, _ := hooks.UseState(initial)
	errs, _ := hooks.UseState(Errors(nil))
	touched, _ := hooks.UseState(map[string]bool{})
	submitting, _ := hooks.UseState(false)
	submitted, _ := hooks.UseState(false)
	f := &Form[T]{Value: value, Errors: errs, Touched: touched, Submitting: submitting, Submitted: submitted,
		parse: map[string]string{}, shown: map[string]*core.Signal[string]{}}
	hooks.OnMount(func() func() {
		return func() {
			if f.cancel != nil {
				f.cancel()
			}
		}
	})
	return f
}

// Validate checks the current value: its Validate method's errors, plus
// fields whose text doesn't parse.
func (f *Form[T]) Validate() Errors {
	v := f.Value.Peek()
	var errs Errors
	if val, ok := any(v).(Validator); ok {
		errs = val.Validate()
	} else if val, ok := any(&v).(Validator); ok {
		errs = val.Validate()
	}
	for name, msg := range f.parse {
		errs = errs.Add(name, msg)
	}
	return errs
}

// Set changes the value through fn and checks it again if errors are
// showing (a field was left or a submit tried).
func (f *Form[T]) Set(fn func(v *T)) {
	v := f.Value.Peek()
	fn(&v)
	core.Batch(func() {
		f.Value.Set(v)
		if f.Submitted.Peek() || len(f.Touched.Peek()) > 0 {
			f.Errors.Set(f.Validate())
		}
	})
}

// Reset starts over from v: no errors, nothing touched or submitted.
func (f *Form[T]) Reset(v T) {
	clear(f.parse)
	core.Batch(func() {
		f.Value.Set(v)
		f.Errors.Set(nil)
		f.Touched.Set(map[string]bool{})
		f.Submitted.Set(false)
	})
}

// touch marks name as left and shows its error.
func (f *Form[T]) touch(name string) {
	if f.Touched.Peek()[name] {
		f.Errors.Set(f.Validate())
		return
	}
	t := map[string]bool{name: true}
	for k := range f.Touched.Peek() {
		t[k] = true
	}
	core.Batch(func() {
		f.Touched.Set(t)
		f.Errors.Set(f.Validate())
	})
}

// Error is name's message while it should show (the field was left, or a
// submit tried), else "". Error("") is the form's own message, such as a
// failed submit's.
func (f *Form[T]) Error(name string) *core.Signal[string] {
	if s, ok := f.shown[name]; ok {
		return s
	}
	s := core.Computed([]core.SignalAccessor{f.Errors, f.Touched, f.Submitted}, func() string {
		if name != "" && !f.Touched.Get()[name] && !f.Submitted.Get() {
			return ""
		}
		return f.Errors.Get()[name]
	})
	f.shown[name] = s
	return s
}

// Invalid is whether name's error is showing.
func (f *Form[T]) Invalid(name string) *core.Signal[bool] {
	e := f.Error(name)
	return core.Computed([]core.SignalAccessor{e}, func() bool { return e.Get() != "" })
}

// OnSubmit handles the form's submit: it shows every error, and if
// Validate finds none, calls submit with the value in a goroutine, with
// Submitting set meanwhile and a context cancelled if the component
// unmounts. An error that carries field messages (FieldErrors, such as
// *rpc.Error) shows them on their fields; another shows as Error(""). Each
// of then runs on the render loop after a successful submit (to navigate, or
// Reset the form). A submit while one is running is ignored.
func (f *Form[T]) OnSubmit(submit func(ctx context.Context, v T) error, then ...func(v T)) core.Item {
	return h.On("submit", func(core.EventData) {
		if f.Submitting.Peek() {
			return
		}
		errs := f.Validate()
		core.Batch(func() {
			f.Submitted.Set(true)
			f.Errors.Set(errs)
		})
		if len(errs) > 0 {
			return
		}
		v := f.Value.Peek()
		ctx, cancel := context.WithCancel(context.Background())
		f.cancel = cancel
		f.Submitting.Set(true)
		go func() {
			err := submit(ctx, v)
			core.Schedule(func() {
				if ctx.Err() != nil {
					return // unmounted
				}
				cancel()
				f.Submitting.Set(false)
				if err == nil {
					f.Errors.Set(nil)
					for _, fn := range then {
						fn(v)
					}
					return
				}
				if fe, ok := err.(FieldErrors); ok && len(fe.FieldErrors()) > 0 {
					errs := Errors(fe.FieldErrors())
					if _, ok := errs[""]; !ok {
						errs = errs.Add("", fe.Error())
					}
					f.Errors.Set(errs)
					return
				}
				f.Errors.Set(Errors{"": err.Error()})
			})
		}()
	}, h.PreventDefault())
}

// Text is a text field (input, textarea) bound to the string field get
// selects: its value, name, aria-invalid, and leaving it (blur) shows its
// error.
func Text[T any](f *Form[T], name string, get func(*T) *string) core.Item {
	value := core.Computed([]core.SignalAccessor{f.Value}, func() string {
		v := f.Value.Get()
		return *get(&v)
	})
	return h.Group(
		h.Name(name),
		h.BindProp("value", value),
		h.OnInputE(func(e core.EventData) {
			s := e.Value()
			f.Set(func(v *T) { *get(v) = s })
		}),
		f.common(name),
	)
}

// Int is a number field bound to the int field get selects. Text that isn't
// a whole number leaves the value as it was and shows "Enter a whole
// number." as the field's error; an empty field is 0.
func Int[T any](f *Form[T], name string, get func(*T) *int) core.Item {
	value := core.Computed([]core.SignalAccessor{f.Value}, func() string {
		v := f.Value.Get()
		return strconv.Itoa(*get(&v))
	})
	return h.Group(
		h.Name(name),
		h.BindProp("value", value),
		h.OnInputE(func(e core.EventData) {
			text := e.Value()
			n, err := strconv.Atoi(text)
			if text == "" {
				n, err = 0, nil
			}
			if err != nil {
				f.parse[name] = "Enter a whole number."
				if f.Submitted.Peek() || f.Touched.Peek()[name] {
					f.Errors.Set(f.Validate())
				}
				return
			}
			delete(f.parse, name)
			f.Set(func(v *T) { *get(v) = n })
		}),
		f.common(name),
	)
}

// Bool is a checkbox bound to the bool field get selects.
func Bool[T any](f *Form[T], name string, get func(*T) *bool) core.Item {
	checked := core.Computed([]core.SignalAccessor{f.Value}, func() bool {
		v := f.Value.Get()
		return *get(&v)
	})
	return h.Group(
		h.Type("checkbox"),
		h.Name(name),
		h.BindProp("checked", checked),
		h.OnChangeE(func(e core.EventData) {
			c := e.Checked()
			f.Set(func(v *T) { *get(v) = c })
		}),
		f.common(name),
	)
}

func (f *Form[T]) common(name string) core.Item {
	return h.Group(
		h.BindAttr("aria-invalid", f.Invalid(name)),
		h.OnBlur(func() { f.touch(name) }),
	)
}
