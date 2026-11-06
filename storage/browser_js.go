//go:build js && wasm

package storage

import (
	"fmt"
	"syscall/js"
)

// storage is the browser's Storage object. Reading window.localStorage throws
// where storage is disabled (a sandboxed frame, a privacy setting), which
// reads as unavailable. The read goes through Reflect.get because js.Value.Get
// doesn't turn a JavaScript exception into a recoverable panic (only calls
// do): a throwing getter would stop the whole program.
func (s *Store) storage() (area js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			area, err = js.Undefined(), ErrUnavailable
		}
	}()
	global := js.Global()
	area = global.Get("Reflect").Call("get", global, s.area)
	if area.IsUndefined() || area.IsNull() {
		return area, ErrUnavailable
	}
	return area, nil
}

func (s *Store) get(key string) (string, bool) {
	area, err := s.storage()
	if err != nil {
		return "", false
	}
	v := area.Call("getItem", key)
	if v.IsNull() {
		return "", false
	}
	return v.String(), true
}

func (s *Store) set(key, value string) (err error) {
	area, err := s.storage()
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("goowee/storage: %v", r) // QuotaExceededError
		}
	}()
	area.Call("setItem", key, value)
	return nil
}

func (s *Store) remove(key string) {
	if area, err := s.storage(); err == nil {
		area.Call("removeItem", key)
	}
}

func (s *Store) clear() {
	if area, err := s.storage(); err == nil {
		area.Call("clear")
	}
}

func (s *Store) keys() []string {
	area, err := s.storage()
	if err != nil {
		return nil
	}
	n := area.Get("length").Int()
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if k := area.Call("key", i); !k.IsNull() {
			keys = append(keys, k.String())
		}
	}
	return keys
}
