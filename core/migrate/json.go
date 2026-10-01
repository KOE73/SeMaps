package migrate

// A tiny ordered-JSON tree. Objects keep their key order; leaf values (strings,
// numbers, booleans, null) stay as json.RawMessage so they are never altered by
// a round trip. The output format matches the repository's own writer
// (core/edit.go encodeDoc): two-space indent, no HTML escaping, trailing newline.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

type obj struct {
	keys []string
	m    map[string]any
}

func newObj() *obj { return &obj{m: map[string]any{}} }

func (o *obj) has(k string) bool { _, ok := o.m[k]; return ok }

// get returns nil for an absent key.
func (o *obj) get(k string) any { return o.m[k] }

func (o *obj) lookup(k string) (any, bool) { v, ok := o.m[k]; return v, ok }

// set replaces the value in place, or appends a new key.
func (o *obj) set(k string, v any) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

func (o *obj) del(k string) {
	if _, ok := o.m[k]; !ok {
		return
	}
	delete(o.m, k)
	for i, x := range o.keys {
		if x == k {
			o.keys = append(o.keys[:i:i], o.keys[i+1:]...)
			break
		}
	}
}

// rename changes a key keeping its position. It refuses (false) when the old
// key is absent or the new one is taken.
func (o *obj) rename(oldK, newK string) bool {
	if !o.has(oldK) || o.has(newK) {
		return false
	}
	for i, x := range o.keys {
		if x == oldK {
			o.keys[i] = newK
			break
		}
	}
	o.m[newK] = o.m[oldK]
	delete(o.m, oldK)
	return true
}

// setFirst puts a key first; an existing key moves to the front with the new value.
func (o *obj) setFirst(k string, v any) {
	if o.has(k) {
		o.del(k)
	}
	o.m[k] = v
	o.keys = append([]string{k}, o.keys...)
}

// insertBefore puts a new key right before an existing one (append when the
// anchor is absent). The key must be new.
func (o *obj) insertBefore(anchor, k string, v any) {
	if o.has(k) {
		o.set(k, v)
		return
	}
	pos := -1
	for i, x := range o.keys {
		if x == anchor {
			pos = i
			break
		}
	}
	o.m[k] = v
	if pos < 0 {
		o.keys = append(o.keys, k)
		return
	}
	keys := make([]string, 0, len(o.keys)+1)
	keys = append(keys, o.keys[:pos]...)
	keys = append(keys, k)
	keys = append(keys, o.keys[pos:]...)
	o.keys = keys
}

func asObj(v any) (*obj, bool)  { o, ok := v.(*obj); return o, ok }
func asArr(v any) ([]any, bool) { a, ok := v.([]any); return a, ok }

func asStr(v any) (string, bool) {
	r, ok := v.(json.RawMessage)
	if !ok || len(r) == 0 || r[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(r, &s) != nil {
		return "", false
	}
	return s, true
}

func isNull(v any) bool { r, ok := v.(json.RawMessage); return ok && string(r) == "null" }

var jsonNull = json.RawMessage("null")

func jsonStr(s string) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return json.RawMessage(bytes.TrimRight(b.Bytes(), "\n"))
}

func jsonInt(n int) json.RawMessage { return json.RawMessage(strconv.Itoa(n)) }

// strField reads an optional string field: (value, present-and-string, error
// when the field holds something that is neither a string nor null).
func strField(o *obj, key string) (string, bool, error) {
	v, ok := o.lookup(key)
	if !ok || isNull(v) {
		return "", false, nil
	}
	s, ok := asStr(v)
	if !ok {
		return "", false, fmt.Errorf("ожидалась строка или null")
	}
	return s, true, nil
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case *obj:
		c := newObj()
		for _, k := range t.keys {
			c.set(k, deepCopy(t.m[k]))
		}
		return c
	case []any:
		c := make([]any, len(t))
		for i, x := range t {
			c[i] = deepCopy(x)
		}
		return c
	case json.RawMessage:
		return append(json.RawMessage(nil), t...)
	}
	return v
}

func splitBOM(b []byte) (bom, body []byte) {
	if bytes.HasPrefix(b, []byte("\xef\xbb\xbf")) {
		return b[:3], b[3:]
	}
	return nil, b
}

func parseTree(data []byte) (any, error) {
	_, body := splitBOM(data)
	var probe json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, err
	}
	return parseValue(probe)
}

func parseValue(raw json.RawMessage) (any, error) {
	b := bytes.TrimSpace(raw)
	if len(b) == 0 {
		return nil, fmt.Errorf("пустое значение")
	}
	switch b[0] {
	case '{':
		return parseObj(b)
	case '[':
		return parseArr(b)
	}
	return json.RawMessage(append([]byte(nil), b...)), nil
}

func parseObj(b []byte) (*obj, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	o := newObj()
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := kt.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		v, err := parseValue(raw)
		if err != nil {
			return nil, err
		}
		o.set(key, v)
	}
	return o, nil
}

func parseArr(b []byte) ([]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	arr := []any{}
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		v, err := parseValue(raw)
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
	}
	return arr, nil
}

func writeValue(b *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case *obj:
		b.WriteByte('{')
		for i, k := range t.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(jsonStr(k))
			b.WriteByte(':')
			if err := writeValue(b, t.m[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, x := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeValue(b, x); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case json.RawMessage:
		b.Write(t)
	case nil:
		b.WriteString("null")
	default:
		return fmt.Errorf("внутренняя ошибка: значение типа %T", v)
	}
	return nil
}

// encodeTree serializes exactly like the repo's writer.
func encodeTree(v any) ([]byte, error) {
	var raw bytes.Buffer
	if err := writeValue(&raw, v); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(json.RawMessage(raw.Bytes())); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// versionOf reads the top-level integer contractVersion.
func versionOf(root *obj) (int, bool, error) {
	v, ok := root.lookup("contractVersion")
	if !ok {
		return 0, false, nil
	}
	r, isRaw := v.(json.RawMessage)
	if !isRaw {
		return 0, true, fmt.Errorf("ожидалось целое число")
	}
	n, err := strconv.Atoi(string(r))
	if err != nil {
		return 0, true, fmt.Errorf("ожидалось целое число, а не %s", string(r))
	}
	return n, true, nil
}

// bumpVersionText replaces the value of the first top-level "contractVersion"
// key with `to`, leaving every other byte alone. changed is false when the key
// is absent or already holds `to`.
func bumpVersionText(orig []byte, to int) (out []byte, changed bool, err error) {
	bom, body := splitBOM(orig)
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return nil, false, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return orig, false, nil
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, false, err
		}
		key, _ := kt.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, false, err
		}
		if key != "contractVersion" {
			continue
		}
		end := int(dec.InputOffset())
		start := end - len(raw)
		want := strconv.Itoa(to)
		if string(raw) == want {
			return orig, false, nil
		}
		res := make([]byte, 0, len(orig)+2)
		res = append(res, bom...)
		res = append(res, body[:start]...)
		res = append(res, want...)
		res = append(res, body[end:]...)
		return res, true, nil
	}
	return orig, false, nil
}
