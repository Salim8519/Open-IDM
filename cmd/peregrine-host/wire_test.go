package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/internal/api"
)

// jsonFields maps JSON names to Go types, flattening embedded structs like encoding/json.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			for k, v := range jsonFields(f.Type) {
				out[k] = v
			}
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = f.Type
	}
	return out
}

// The host's own message types must stay a subset of the app's, or captures from the
// browser would silently lose fields.
func TestWireTypesMatchApp(t *testing.T) {
	cases := []struct {
		name      string
		host, app reflect.Type
	}{
		{"request", reflect.TypeOf(wireRequest{}), reflect.TypeOf(engine.Request{})},
		{"capture", reflect.TypeOf(wireCapture{}), reflect.TypeOf(api.Capture{})},
	}
	for _, c := range cases {
		app := jsonFields(c.app)
		for name, typ := range jsonFields(c.host) {
			if got, ok := app[name]; !ok {
				t.Errorf("%s: field %q is not in the app's type", c.name, name)
			} else if got != typ {
				t.Errorf("%s: field %q is %v in the host but %v in the app", c.name, name, typ, got)
			}
		}
	}
}
