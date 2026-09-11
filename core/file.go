package core

import (
	"errors"
	"sync"
	"time"
)

// File describes one file picked through an <input type="file">, as returned
// by EventData.Files on a change/input event. The metadata travels with the
// event; the contents stay in the browser until Bytes reads them.
type File struct {
	Name         string
	Size         int64
	Type         string // MIME type as reported by the browser; "" when unknown
	LastModified time.Time

	handle int // the bridge's key for the live File object; 0 = not readable
}

// Files returns the files selected on a file input. Empty for any other
// element or event type.
func (e EventData) Files() []File {
	raw, ok := e.Data["files"].([]any)
	if !ok {
		return nil
	}
	out := make([]File, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		f := File{
			Name: mapStr(m, "name"),
			Size: int64(mapNum(m, "size")),
			Type: mapStr(m, "type"),
		}
		if ms := mapNum(m, "lastModified"); ms != 0 {
			f.LastModified = time.UnixMilli(int64(ms))
		}
		f.handle = int(mapNum(m, "handle"))
		out = append(out, f)
	}
	return out
}

// ErrNoFileReader is returned by File.Bytes when no reader is installed —
// during SSR, or in tests without a running client.
var ErrNoFileReader = errors.New("goowee: file contents are only readable in the browser")

var (
	fileReaderMu sync.Mutex
	fileReader   func(handle int) ([]byte, error)
)

// SetFileReader installs the function File.Bytes uses to fetch a file's
// contents by handle. The WASM bridge sets it at startup (see bridge.Run);
// SSR and tests leave it unset.
func SetFileReader(fn func(handle int) ([]byte, error)) {
	fileReaderMu.Lock()
	fileReader = fn
	fileReaderMu.Unlock()
}

// Bytes reads the file's contents from the browser. It blocks until the read
// completes, so call it from a goroutine — never inline in an event handler,
// where blocking would deadlock the WASM event loop — and apply the result on
// the render loop with Schedule, the same rule as any fetch:
//
//	OnChangeE(func(e core.EventData) {
//	    for _, f := range e.Files() {
//	        go func() {
//	            data, err := f.Bytes()
//	            core.Schedule(func() { /* Set signals from data/err */ })
//	        }()
//	    }
//	})
//
// The file stays readable until the input's selection changes or the element
// is removed; after that Bytes returns an error. Outside the browser it
// returns ErrNoFileReader.
func (f File) Bytes() ([]byte, error) {
	fileReaderMu.Lock()
	read := fileReader
	fileReaderMu.Unlock()
	if read == nil {
		return nil, ErrNoFileReader
	}
	if f.handle == 0 {
		return nil, errors.New("goowee: file is not readable (no handle)")
	}
	return read(f.handle)
}
