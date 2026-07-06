package h

import (
	"github.com/yogisalomo/goowee/core"
	"strconv"
)

func Class(v string) core.Item           { return attrItem{"class", v} }
func ID(v string) core.Item              { return attrItem{"id", v} }
func Href(v string) core.Item            { return attrItem{"href", v} }
func Src(v string) core.Item             { return attrItem{"src", v} }
func Alt(v string) core.Item             { return attrItem{"alt", v} }
func Title(v string) core.Item           { return attrItem{"title", v} }
func Style(v string) core.Item           { return attrItem{"style", v} }
func Placeholder(v string) core.Item     { return attrItem{"placeholder", v} }
func Name(v string) core.Item            { return attrItem{"name", v} }
func Type(v string) core.Item            { return attrItem{"type", v} }
func HtmlFor(v string) core.Item         { return attrItem{"for", v} }
func Rel(v string) core.Item             { return attrItem{"rel", v} }
func Target(v string) core.Item          { return attrItem{"target", v} }
func Role(v string) core.Item            { return attrItem{"role", v} }
func Action(v string) core.Item          { return attrItem{"action", v} }
func Method(v string) core.Item          { return attrItem{"method", v} }
func Autocomplete(v string) core.Item    { return attrItem{"autocomplete", v} }
func Min(v string) core.Item             { return attrItem{"min", v} }
func Max(v string) core.Item             { return attrItem{"max", v} }
func Step(v string) core.Item            { return attrItem{"step", v} }
func Pattern(v string) core.Item         { return attrItem{"pattern", v} }
func Accept(v string) core.Item          { return attrItem{"accept", v} }
func Width(v string) core.Item           { return attrItem{"width", v} }
func Height(v string) core.Item          { return attrItem{"height", v} }
func AriaLabel(v string) core.Item       { return attrItem{"aria-label", v} }
func AriaHidden(v bool) core.Item        { return attrItem{"aria-hidden", fmtBool(v)} }
func AriaExpanded(v bool) core.Item      { return attrItem{"aria-expanded", fmtBool(v)} }
func AriaCurrent(v string) core.Item     { return attrItem{"aria-current", v} }
func AriaDescribedBy(v string) core.Item { return attrItem{"aria-describedby", v} }
func AriaLabelledBy(v string) core.Item  { return attrItem{"aria-labelledby", v} }
func AriaControls(v string) core.Item    { return attrItem{"aria-controls", v} }
func AriaPressed(v bool) core.Item       { return attrItem{"aria-pressed", fmtBool(v)} }
func AriaSelected(v bool) core.Item      { return attrItem{"aria-selected", fmtBool(v)} }
func AriaInvalid(v bool) core.Item       { return attrItem{"aria-invalid", fmtBool(v)} }
func AriaRequired(v bool) core.Item      { return attrItem{"aria-required", fmtBool(v)} }
func AriaHaspopup(v string) core.Item    { return attrItem{"aria-haspopup", v} }
func AriaModal(v bool) core.Item         { return attrItem{"aria-modal", fmtBool(v)} }
func AriaLive(v string) core.Item        { return attrItem{"aria-live", v} }
func AriaAtomic(v bool) core.Item        { return attrItem{"aria-atomic", fmtBool(v)} }
func AriaBusy(v bool) core.Item          { return attrItem{"aria-busy", fmtBool(v)} }

func Rows(v int) core.Item     { return attrItem{"rows", strconv.Itoa(v)} }
func Cols(v int) core.Item     { return attrItem{"cols", strconv.Itoa(v)} }
func TabIndex(v int) core.Item { return attrItem{"tabindex", strconv.Itoa(v)} }
func Colspan(v int) core.Item  { return attrItem{"colspan", strconv.Itoa(v)} }
func Rowspan(v int) core.Item  { return attrItem{"rowspan", strconv.Itoa(v)} }

func Attr(name, value string) core.Item { return attrItem{name, value} }
func Data(name, value string) core.Item { return attrItem{"data-" + name, value} }
func Aria(name, value string) core.Item { return attrItem{"aria-" + name, value} }

func Value(v string) core.Item  { return propItem{"value", v} }
func Checked(v bool) core.Item  { return propItem{"checked", v} }
func Disabled(v bool) core.Item { return propItem{"disabled", v} }
func Selected(v bool) core.Item { return propItem{"selected", v} }
func ReadOnly(v bool) core.Item { return propItem{"readOnly", v} }
func Multiple(v bool) core.Item { return propItem{"multiple", v} }
func Required(v bool) core.Item { return propItem{"required", v} }

func AutoFocus(v bool) core.Item { return propItem{"autofocus", v} }

func Prop(name string, v any) core.Item { return propItem{name, v} }

func fmtBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
