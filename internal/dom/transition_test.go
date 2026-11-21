package dom

import (
	"testing"

	. "github.com/yogisalomo/goowee/h"
)

func TestTransitionNamesFindsNestedRootsInOneWalk(t *testing.T) {
	row := Div(ID("row"), Transition("fade"), Text("row"))
	row.ID = 1 // runtime node id, assigned by the walker in a real render
	plain := Div(ID("plain"), Text("plain"))
	plain.ID = 2
	tree := Div(
		Fragment(row),
		Fragment(plain),
	)

	names := make(map[int]string)
	transitionNames(tree, names)
	if got := names[row.ID]; got != "fade" {
		t.Fatalf("transition for row = %q, want fade", got)
	}
	if got := names[plain.ID]; got != "" {
		t.Fatalf("transition for plain row = %q, want none", got)
	}
	if len(names) != 1 {
		t.Fatalf("names = %v, want only the transitioned row", names)
	}
}
