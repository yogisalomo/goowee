# Forms

`goowee/form` keeps a form's fields in one typed value, checked by a plain Go
method that the browser and the server both run.

## The value and its check

```go
type Signup struct {
	Email string
	Age   int
}

func (s Signup) Validate() form.Errors {
	var errs form.Errors
	if !strings.Contains(s.Email, "@") {
		errs = errs.Add("email", "Enter an email address.")
	}
	if s.Age < 13 {
		errs = errs.Add("age", "You must be 13 or older.")
	}
	return errs
}
```

Put it in a package both sides import (an `api` package, say). It is plain
Go: no tags, no reflection.

## The form

```go
f := form.Use(Signup{})
return Form(
	f.OnSubmit(func(ctx context.Context, v Signup) error {
		_, err := api.Signup.Call(ctx, v)
		return err
	}, func(v Signup) { r.Navigate("/welcome") }),
	Label(Text("Email"), Input(Type("email"), form.Text(f, "email", func(s *Signup) *string { return &s.Email }))),
	P(Class("field-error"), TextS(f.Error("email"))),
	Label(Text("Age"), Input(Type("number"), form.Int(f, "age", func(s *Signup) *int { return &s.Age }))),
	P(Class("field-error"), TextS(f.Error("age"))),
	P(Class("form-error"), TextS(f.Error(""))),
	Button(Type("submit"), DisabledS(f.Submitting), Text("Sign up")),
)
```

- `form.Text`, `form.Int` and `form.Bool` (a checkbox) bind an input to one
  field in both directions. They also set its `name`, keep `aria-invalid`
  current, and mark the field as touched when the user leaves it. The
  accessor function picks the field without reflection.
- A field's error shows once it was left, or once a submit was tried. After
  that, `Validate` runs again on every change, so the message goes away as
  soon as the input is fixed. `form.Int` reports text that isn't a whole
  number itself.
- `f.OnSubmit(send, then...)` shows every error and sends nothing while
  `Validate` finds any. Otherwise it calls `send` in a goroutine with
  `f.Submitting` set, under a context that is cancelled if the component
  unmounts. A second submit while one runs is ignored.
  - If `send` returns an error with field messages (`*rpc.Error` from
    `rpc.Invalid`, or any error with a `FieldErrors() map[string]string`
    method), each message shows on its field.
  - Any other error shows as `f.Error("")`.
  - After a success, each `then` runs on the render loop (to navigate, or
    `f.Reset` the form).
- `f.Value`, `f.Errors`, `f.Touched`, `f.Submitting` and `f.Submitted` are
  signals, for anything else the view needs. `f.Set(func(v *Signup) {…})`
  changes the value from code.

## The server runs the same check

```go
rpc.Handle(mux, api.Signup, func(ctx context.Context, s api.SignupReq) (api.User, error) {
	if errs := s.Validate(); len(errs) > 0 {
		return api.User{}, rpc.Invalid("check the form", errs)
	}
	if taken(s.Email) { // what only the server knows
		return api.User{}, rpc.Invalid("check the form", map[string]string{"email": "That address is taken."})
	}
	…
})
```

`examples/notes` does exactly this for its new-note form.

## Testing

gooweetest drives a form like a user would. Use `Input` to type, `Dispatch`
with `"blur"` to leave a field, and `Submit` to submit. For the submit's
goroutine, use `WaitForText` or `WaitFor`. `form/form_test.go` is a full
example, including a server error.
