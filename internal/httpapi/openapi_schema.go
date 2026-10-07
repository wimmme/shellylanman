package httpapi

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"github.com/wimmme/shellylanman/internal/logbuf"
	"github.com/wimmme/shellylanman/internal/model"
	"github.com/wimmme/shellylanman/internal/update"
)

// schemaGen turns the Go types the handlers use into JSON Schema (OpenAPI 3.1),
// so the description cannot drift from the code: named structs become
// components, field names and "omitempty" come from the json tags.
type schemaGen struct {
	comps map[string]any
	names map[reflect.Type]string
	taken map[string]reflect.Type
}

func newSchemaGen() *schemaGen {
	return &schemaGen{comps: map[string]any{}, names: map[reflect.Type]string{}, taken: map[string]reflect.Type{}}
}

var (
	rawMessage = reflect.TypeOf(json.RawMessage(nil))
	// component names where the Go name would clash or say too little.
	schemaNames = map[reflect.Type]string{
		reflect.TypeOf(logbuf.Entry{}):  "LogEntry",
		reflect.TypeOf(update.Status{}): "UpdateStatus",
	}
	// enums of named string types.
	enums = map[reflect.Type][]string{
		reflect.TypeOf(model.Status("")): {"online", "offline", "login", "reading", "error", "ghost", "searching"},
	}
)

// of returns the schema of a value's type; for a named struct a $ref.
func (g *schemaGen) of(v any) map[string]any { return g.schema(reflect.TypeOf(v)) }

func (g *schemaGen) schema(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == rawMessage {
		return map[string]any{} // any JSON value
	}
	if e, ok := enums[t]; ok {
		return map[string]any{"type": "string", "enum": e}
	}
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return map[string]any{"type": "integer"}
	case reflect.Int64, reflect.Uint64:
		return map[string]any{"type": "integer", "format": "int64"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 { // []byte: base64
			return map[string]any{"type": "string", "contentEncoding": "base64"}
		}
		return map[string]any{"type": "array", "items": g.schema(t.Elem())}
	case reflect.Array:
		return map[string]any{"type": "array", "items": g.schema(t.Elem()), "minItems": t.Len(), "maxItems": t.Len()}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case reflect.Struct:
		if t.Name() == "" {
			return g.object(t, "")
		}
		return g.ref(t)
	}
	return map[string]any{}
}

// ref registers a named struct as a component and returns the reference.
func (g *schemaGen) ref(t reflect.Type) map[string]any {
	name, ok := g.names[t]
	if !ok {
		name = t.Name()
		if n, ok := schemaNames[t]; ok {
			name = n
		}
		if other, clash := g.taken[name]; clash && other != t {
			pkg := t.PkgPath()
			pkg = pkg[strings.LastIndex(pkg, "/")+1:]
			name = strings.ToUpper(pkg[:1]) + pkg[1:] + name
		}
		g.names[t] = name
		g.taken[name] = t
		g.comps[name] = nil // reserved: recursive types find the name
		g.comps[name] = g.object(t, name)
	}
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

type fieldInfo struct {
	name     string
	optional bool
	typ      reflect.Type
}

func jsonFields(t reflect.Type) []fieldInfo {
	var out []fieldInfo
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" || (!f.IsExported() && !f.Anonymous) {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" && (f.Type.Kind() == reflect.Struct) { // embedded struct: its fields are ours
			out = append(out, jsonFields(f.Type)...)
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		optional := strings.Contains(opts, "omitempty") || f.Type.Kind() == reflect.Pointer
		out = append(out, fieldInfo{name: name, optional: optional, typ: f.Type})
	}
	return out
}

// object is the schema of a struct. Descriptions of fields come from fieldDocs.
func (g *schemaGen) object(t reflect.Type, name string) map[string]any {
	props := map[string]any{}
	var required []string
	for _, f := range jsonFields(t) {
		s := g.schema(f.typ)
		if d, ok := fieldDocs[name+"."+f.name]; ok {
			if _, isRef := s["$ref"]; isRef { // a description next to a $ref goes in allOf
				s = map[string]any{"allOf": []any{s}, "description": d}
			} else {
				s["description"] = d
			}
		}
		props[f.name] = s
		if !f.optional {
			required = append(required, f.name)
		}
	}
	o := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		sort.Strings(required)
		o["required"] = required
	}
	if d, ok := typeDocs[name]; ok {
		o["description"] = d
	}
	return o
}

// typeFields lists the json names of a struct type (tests compare them with the description).
func typeFields(t reflect.Type) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	var names []string
	for _, f := range jsonFields(t) {
		names = append(names, f.name)
	}
	sort.Strings(names)
	return names
}
