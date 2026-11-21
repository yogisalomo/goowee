package form_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yogisalomo/goowee/core"
	"github.com/yogisalomo/goowee/form"
	"github.com/yogisalomo/goowee/gooweetest"
	. "github.com/yogisalomo/goowee/h"
)

type signup struct {
	Email  string
	Age    int
	Agreed bool
}

func (s signup) Validate() form.Errors {
	var errs form.Errors
	if !strings.Contains(s.Email, "@") {
		errs = errs.Add("Email", "Enter an email address.")
	}
	if s.Age < 13 {
		errs = errs.Add("Age", "You must be 13 or older.")
	}
	if !s.Agreed {
		errs = errs.Add("Agreed", "Accept the terms.")
	}
	return errs
}

// fieldErr is what a server returns for a field (as *rpc.Error does).
type fieldErr map[string]string

func (e fieldErr) Error() string                  { return "check the form" }
func (e fieldErr) FieldErrors() map[string]string { return e }

func view(submit func(context.Context, signup) error, done *[]signup) core.Node {
	return core.Component("Signup", func() core.Node {
		f := form.Use(signup{})
		return Form(ID("f"),
			f.OnSubmit(submit, func(v signup) {
				*done = append(*done, v)
				f.Reset(signup{})
			}),
			Input(ID("email"), form.Text(f, "Email", func(s *signup) *string { return &s.Email })),
			P(ID("email-error"), TextS(f.Error("Email"))),
			Input(ID("age"), Type("number"), form.Int(f, "Age", func(s *signup) *int { return &s.Age })),
			P(ID("age-error"), TextS(f.Error("Age"))),
			Input(ID("agreed"), form.Bool(f, "Agreed", func(s *signup) *bool { return &s.Agreed })),
			P(ID("form-error"), TextS(f.Error(""))),
			Button(ID("go"), Type("submit"), DisabledS(f.Submitting), Text("Sign up")),
		)
	})
}

func TestFormTypeBlurSubmitServerError(t *testing.T) {
	release := make(chan error)
	var sent []signup
	submit := func(ctx context.Context, v signup) error { return <-release }
	s := gooweetest.Render(t, view(submit, &sent))
	email, age, agreed := s.Find("#email"), s.Find("#age"), s.Find("#agreed")

	// Typing doesn't show errors; leaving the field does.
	s.Input(email, "ada")
	if got := s.Find("#email-error").Text(); got != "" {
		t.Fatalf("an error before the field was left: %q", got)
	}
	s.Dispatch(email, "blur", nil)
	if got := s.Find("#email-error").Text(); got != "Enter an email address." {
		t.Fatalf("after blur: %q", got)
	}
	if email.Attr("aria-invalid") != "true" {
		t.Fatalf("aria-invalid %q", email.Attr("aria-invalid"))
	}
	// Each change after that checks again.
	s.Input(email, "ada@example.com")
	if got := s.Find("#email-error").Text(); got != "" || email.Attr("aria-invalid") != "false" {
		t.Fatalf("after fixing: %q, aria-invalid %q", got, email.Attr("aria-invalid"))
	}
	// A submit with errors shows them all and sends nothing.
	if !s.Submit(s.Find("#f")) {
		t.Fatal("the submit's default wasn't prevented")
	}
	if got := s.Find("#age-error").Text(); got != "You must be 13 or older." {
		t.Fatalf("age error after submit: %q", got)
	}
	// Text that isn't a number keeps the value and says so.
	s.Input(age, "1x")
	if got := s.Find("#age-error").Text(); got != "Enter a whole number." {
		t.Fatalf("parse error: %q", got)
	}
	s.Input(age, "36")
	s.Check(agreed, true)
	if got := s.Find("#age-error").Text(); got != "" {
		t.Fatalf("age error after fixing: %q", got)
	}

	// A valid submit runs, with Submitting set; the server rejects a field.
	s.Submit(s.Find("#f"))
	s.Flush()
	if !s.Find("#go").Prop("disabled").(bool) {
		t.Fatal("the button isn't disabled while submitting")
	}
	s.Submit(s.Find("#f")) // ignored while one runs
	release <- fieldErr{"Email": "That address is taken."}
	s.WaitForText("That address is taken.")
	if got := s.Find("#form-error").Text(); got != "check the form" {
		t.Fatalf("form error: %q", got)
	}
	if s.Find("#go").Prop("disabled").(bool) {
		t.Fatal("the button stays disabled")
	}

	// A plain error shows on the form; success runs then, which resets it.
	s.Submit(s.Find("#f"))
	release <- errors.New("network down")
	s.WaitForText("network down")
	s.Submit(s.Find("#f"))
	release <- nil
	s.WaitFor(func() bool { return len(sent) == 1 })
	if sent[0] != (signup{"ada@example.com", 36, true}) {
		t.Fatalf("sent %+v", sent[0])
	}
	s.Flush()
	if v := email.Prop("value"); v != "" {
		t.Fatalf("Reset left %q", v)
	}
	if got := s.Find("#email-error").Text() + s.Find("#form-error").Text(); got != "" {
		t.Fatalf("errors after Reset: %q", got)
	}
}

// Unmounting cancels a running submit, and its result is dropped.
func TestFormSubmitCancelledOnUnmount(t *testing.T) {
	cancelled := make(chan bool, 1)
	submit := func(ctx context.Context, v signup) error {
		<-ctx.Done()
		cancelled <- true
		return ctx.Err()
	}
	var sent []signup
	s := gooweetest.Render(t, view(submit, &sent))
	s.Input(s.Find("#email"), "a@b")
	s.Input(s.Find("#age"), "20")
	s.Check(s.Find("#agreed"), true)
	s.Submit(s.Find("#f"))
	s.Unmount()
	if !<-cancelled {
		t.Fatal("not cancelled")
	}
}
