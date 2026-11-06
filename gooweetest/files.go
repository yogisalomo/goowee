package gooweetest

import (
	"errors"
	"time"
)

// File is a file a test gives the app through Paste, Drop or SelectFiles.
// The app sees it in EventData.Files, and File.Bytes returns Data.
type File struct {
	Name         string
	Type         string // MIME type, e.g. "image/png"
	Data         []byte
	LastModified time.Time
}

// Paste fires a paste on el carrying text and files, as pasting from the
// clipboard does (a screenshot arrives as one image file and no text). It
// reports whether a handler called PreventDefault; a paste nobody prevented
// isn't inserted into el's value here, since the browser would do that.
func (s *Screen) Paste(el *Element, text string, files ...File) (prevented bool) {
	s.t.Helper()
	return s.Dispatch(el, "paste", s.transfer(text, files))
}

// Drop fires a drop on el carrying files, as dropping files from the desktop
// does, and reports whether a handler called PreventDefault. (A real drop
// only reaches handlers once a dragover handler accepts it; dispatch
// "dragover" with DragOverData to test that part.)
func (s *Screen) Drop(el *Element, files ...File) (prevented bool) {
	s.t.Helper()
	return s.Dispatch(el, "drop", s.transfer("", files))
}

// DragOverData is the event data of a dragover carrying files, for Dispatch:
// the browser exposes their presence ("Files" in DataTypes) but not the files
// until the drop.
func DragOverData() map[string]any {
	return map[string]any{"types": []any{"Files"}}
}

// SelectFiles picks files on a file input and fires input and change, as
// choosing them in the file dialog does.
func (s *Screen) SelectFiles(input *Element, files ...File) {
	s.t.Helper()
	data := s.transfer("", files)
	data["value"] = ""
	if len(files) > 0 {
		data["value"] = `C:\fakepath\` + files[0].Name // what browsers report
	}
	delete(data, "types")
	s.Dispatch(input, "input", data)
	s.Dispatch(input, "change", data)
}

// transfer builds a DataTransfer payload, registering each file's contents
// under a handle for File.Bytes.
func (s *Screen) transfer(text string, files []File) map[string]any {
	var types []any
	if text != "" {
		types = append(types, "text/plain")
	}
	list := make([]any, 0, len(files))
	s.filesMu.Lock()
	for _, f := range files {
		h := len(s.files) + 1
		s.files[h] = f.Data
		item := map[string]any{"handle": float64(h), "name": f.Name, "size": float64(len(f.Data)), "type": f.Type}
		if !f.LastModified.IsZero() {
			item["lastModified"] = float64(f.LastModified.UnixMilli())
		}
		list = append(list, item)
	}
	s.filesMu.Unlock()
	if len(files) > 0 {
		types = append(types, "Files")
	}
	return map[string]any{"text": text, "types": types, "files": list}
}

func (s *Screen) readFile(handle int) ([]byte, error) {
	s.filesMu.Lock()
	defer s.filesMu.Unlock()
	data, ok := s.files[handle]
	if !ok {
		return nil, errors.New("gooweetest: no such file")
	}
	return append([]byte(nil), data...), nil
}
