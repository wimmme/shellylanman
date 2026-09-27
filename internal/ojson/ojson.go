// Package ojson is a small ordered JSON document model for backup and
// restore. Object keys keep their order and numbers keep their literal text,
// so Gen1 restores send parameters in the order of the backup (as Jackson's
// ObjectNode does in ShellyScanner) and values are not reformatted.
//
// Accessors are forgiving like Jackson's path(): a missing key yields a
// missing value, never nil.
package ojson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Kind of a value.
type Kind int

// Kinds.
const (
	Missing Kind = iota
	Null
	Bool
	Number
	String
	Array
	Object
)

// Value is one JSON value.
type Value struct {
	kind Kind
	b    bool
	s    string // string value or number literal
	arr  []*Value
	keys []string
	m    map[string]*Value
}

var missing = &Value{kind: Missing}

// Parse decodes b.
func Parse(b []byte) (*Value, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	v, err := parse(dec)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// MustParse is Parse for literals in code and tests.
func MustParse(s string) *Value {
	v, err := Parse([]byte(s))
	if err != nil {
		panic(err)
	}
	return v
}

func parse(dec *json.Decoder) (*Value, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch x := t.(type) {
	case json.Delim:
		switch x {
		case '{':
			o := NewObject()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				v, err := parse(dec)
				if err != nil {
					return nil, err
				}
				o.Set(k, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return o, nil
		case '[':
			a := NewArray()
			for dec.More() {
				v, err := parse(dec)
				if err != nil {
					return nil, err
				}
				a.arr = append(a.arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return a, nil
		}
	case string:
		return Str(x), nil
	case json.Number:
		return &Value{kind: Number, s: string(x)}, nil
	case bool:
		return BoolV(x), nil
	case nil:
		return NullV(), nil
	}
	return nil, fmt.Errorf("ojson: unexpected token %v", t)
}

// Constructors.
func NewObject() *Value       { return &Value{kind: Object, m: map[string]*Value{}} }
func NewArray() *Value        { return &Value{kind: Array} }
func Str(s string) *Value     { return &Value{kind: String, s: s} }
func BoolV(b bool) *Value     { return &Value{kind: Bool, b: b} }
func NullV() *Value           { return &Value{kind: Null} }
func Int(i int) *Value        { return &Value{kind: Number, s: strconv.Itoa(i)} }
func Num(lit string) *Value   { return &Value{kind: Number, s: lit} }
func Missed() *Value          { return missing }
func (v *Value) Kind() Kind   { return v.kind }
func (v *Value) Exists() bool { return v != nil && v.kind != Missing }

// IsNull: JSON null (not a missing key).
func (v *Value) IsNull() bool { return v != nil && v.kind == Null }

// NonNull: present and not null (Jackson hasNonNull on the parent).
func (v *Value) NonNull() bool { return v.Exists() && v.kind != Null }

// Get returns a member of an object, or a missing value.
func (v *Value) Get(key string) *Value {
	if v == nil || v.kind != Object {
		return missing
	}
	if c, ok := v.m[key]; ok {
		return c
	}
	return missing
}

// Path walks keys.
func (v *Value) Path(keys ...string) *Value {
	for _, k := range keys {
		v = v.Get(k)
	}
	return v
}

// Idx returns an array element, or a missing value.
func (v *Value) Idx(i int) *Value {
	if v == nil || v.kind != Array || i < 0 || i >= len(v.arr) {
		return missing
	}
	return v.arr[i]
}

// Len of an array or object.
func (v *Value) Len() int {
	switch {
	case v == nil:
		return 0
	case v.kind == Array:
		return len(v.arr)
	case v.kind == Object:
		return len(v.keys)
	}
	return 0
}

// Items of an array.
func (v *Value) Items() []*Value {
	if v == nil || v.kind != Array {
		return nil
	}
	return v.arr
}

// Keys of an object in document order.
func (v *Value) Keys() []string {
	if v == nil || v.kind != Object {
		return nil
	}
	return append([]string(nil), v.keys...)
}

// Text is Jackson's asString(""): the string, the number literal, "true" /
// "false"; "" for null, missing, arrays and objects.
func (v *Value) Text() string {
	if v == nil {
		return ""
	}
	switch v.kind {
	case String, Number:
		return v.s
	case Bool:
		return strconv.FormatBool(v.b)
	}
	return ""
}

// Str returns the string value, or def when it is not a string.
func (v *Value) Str(def string) string {
	if v != nil && v.kind == String {
		return v.s
	}
	return def
}

// Bool is Jackson's asBoolean(): true for true, "true" and non-zero numbers.
func (v *Value) Bool() bool {
	if v == nil {
		return false
	}
	switch v.kind {
	case Bool:
		return v.b
	case String:
		return v.s == "true"
	case Number:
		f, _ := strconv.ParseFloat(v.s, 64)
		return f != 0
	}
	return false
}

// Float of a number (or numeric string).
func (v *Value) Float() float64 {
	if v == nil || (v.kind != Number && v.kind != String) {
		return 0
	}
	f, _ := strconv.ParseFloat(v.s, 64)
	return f
}

// Int of a number (truncated).
func (v *Value) Int() int { return int(v.Float()) }

// Set adds or replaces a member, keeping the position of an existing key.
func (v *Value) Set(key string, val *Value) *Value {
	if v.kind != Object {
		panic("ojson: Set on non-object")
	}
	if val == nil {
		val = NullV()
	}
	if _, ok := v.m[key]; !ok {
		v.keys = append(v.keys, key)
	}
	v.m[key] = val
	return v
}

// Remove deletes a member and returns it (missing when absent).
func (v *Value) Remove(key string) *Value {
	if v == nil || v.kind != Object {
		return missing
	}
	old, ok := v.m[key]
	if !ok {
		return missing
	}
	delete(v.m, key)
	for i, k := range v.keys {
		if k == key {
			v.keys = append(v.keys[:i], v.keys[i+1:]...)
			break
		}
	}
	return old
}

// Append adds an element to an array.
func (v *Value) Append(val *Value) *Value {
	if v.kind != Array {
		panic("ojson: Append on non-array")
	}
	v.arr = append(v.arr, val)
	return v
}

// SetIdx replaces an array element.
func (v *Value) SetIdx(i int, val *Value) {
	if v.kind == Array && i >= 0 && i < len(v.arr) {
		v.arr[i] = val
	}
}

// Clone is a deep copy (Jackson deepCopy).
func (v *Value) Clone() *Value {
	if v == nil || v.kind == Missing {
		return missing
	}
	c := &Value{kind: v.kind, b: v.b, s: v.s}
	switch v.kind {
	case Array:
		for _, x := range v.arr {
			c.arr = append(c.arr, x.Clone())
		}
	case Object:
		c.m = map[string]*Value{}
		for _, k := range v.keys {
			c.keys = append(c.keys, k)
			c.m[k] = v.m[k].Clone()
		}
	}
	return c
}

// Equal compares like Jackson JsonNode.equals: object members regardless of
// order, numbers by value.
func Equal(a, b *Value) bool {
	if !a.Exists() || !b.Exists() {
		return a.Exists() == b.Exists()
	}
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case Null:
		return true
	case Bool:
		return a.b == b.b
	case String:
		return a.s == b.s
	case Number:
		return a.Float() == b.Float()
	case Array:
		if len(a.arr) != len(b.arr) {
			return false
		}
		for i := range a.arr {
			if !Equal(a.arr[i], b.arr[i]) {
				return false
			}
		}
		return true
	case Object:
		if len(a.keys) != len(b.keys) {
			return false
		}
		for _, k := range a.keys {
			bv, ok := b.m[k]
			if !ok || !Equal(a.m[k], bv) {
				return false
			}
		}
		return true
	}
	return false
}

// MarshalJSON writes the value with its key order; a missing value is null.
func (v *Value) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	v.write(&buf)
	return buf.Bytes(), nil
}

// String is the compact JSON text.
func (v *Value) String() string {
	b, _ := v.MarshalJSON()
	return string(b)
}

func (v *Value) write(buf *bytes.Buffer) {
	if v == nil {
		buf.WriteString("null")
		return
	}
	switch v.kind {
	case Missing, Null:
		buf.WriteString("null")
	case Bool:
		buf.WriteString(strconv.FormatBool(v.b))
	case Number:
		buf.WriteString(v.s)
	case String:
		b, _ := json.Marshal(v.s)
		buf.Write(b)
	case Array:
		buf.WriteByte('[')
		for i, x := range v.arr {
			if i > 0 {
				buf.WriteByte(',')
			}
			x.write(buf)
		}
		buf.WriteByte(']')
	case Object:
		buf.WriteByte('{')
		for i, k := range v.keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			b, _ := json.Marshal(k)
			buf.Write(b)
			buf.WriteByte(':')
			v.m[k].write(buf)
		}
		buf.WriteByte('}')
	}
}

// Obj builds an object from key/value pairs; values may be *Value, string,
// bool, int, float64 or nil.
func Obj(kv ...any) *Value {
	o := NewObject()
	for i := 0; i+1 < len(kv); i += 2 {
		o.Set(kv[i].(string), From(kv[i+1]))
	}
	return o
}

// From converts a Go value to a Value.
func From(x any) *Value {
	switch t := x.(type) {
	case *Value:
		return t
	case nil:
		return NullV()
	case string:
		return Str(t)
	case bool:
		return BoolV(t)
	case int:
		return Int(t)
	case float64:
		return Num(strconv.FormatFloat(t, 'f', -1, 64))
	case float32:
		return Num(strconv.FormatFloat(float64(t), 'f', -1, 32))
	}
	b, _ := json.Marshal(x)
	v, err := Parse(b)
	if err != nil {
		return NullV()
	}
	return v
}
