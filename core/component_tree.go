package core

type ComponentFrame struct {
	Path        string
	Parent      *ComponentFrame
	Children    []*ComponentFrame
	RootNodeIDs []int
	Hooks       []any
	Disposers   []func()
}

var renderStack []*ComponentFrame

func PushComponent() *ComponentFrame {
	parent := CurrentComponent()
	path := "/"
	if parent != nil {
		path = parent.Path + "/" + itoa(len(parent.Children))
	}
	node := &ComponentFrame{
		Path:   path,
		Parent: parent,
	}
	if parent != nil {
		parent.Children = append(parent.Children, node)
	}
	renderStack = append(renderStack, node)
	return node
}

func PopComponent() {
	renderStack = renderStack[:len(renderStack)-1]
}

func CurrentComponent() *ComponentFrame {
	if len(renderStack) == 0 {
		return nil
	}
	return renderStack[len(renderStack)-1]
}

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
