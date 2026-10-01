package core

import (
	"encoding/json"
	"strings"
)

// TextMark names one text field a batch touched.
type TextMark struct {
	Key   string `json:"key"`
	Field string `json:"field"`
}

// fieldValue reads one field of a text entry: its text and its provenance.
// ok is false when the field is absent or not a value with provenance.
func fieldValue(entry *object, field string) (v textValue, ok bool) {
	raw, present := entry.vals[field]
	if !present || len(raw) == 0 || raw[0] != '{' {
		return textValue{}, false
	}
	if json.Unmarshal(raw, &v) != nil {
		return textValue{}, false
	}
	return v, true
}

// translatedValue is the value {v, at, origin: translated, from, fromHash} of
// text, a translation of source: the hash is core.Hash of the source text, the
// one `semaps check` verifies (CONTRACT §7.3).
func translatedValue(text, from, sourceText string) *object {
	v := newObject()
	v.set("v", text)
	v.set("at", now().Format("2006-01-02T15:04:05Z"))
	v.set("origin", "translated")
	v.set("from", from)
	v.set("fromHash", Hash(sourceText))
	return v
}

// SetTextFrom writes one field as a translation of the same field in language
// from: origin translated, from, and the hash of the source text as it is now.
// Refused when from is lang or the source text does not exist.
func (m *Model) SetTextFrom(lang, key, field, value, from, author string) error {
	if from == lang {
		return refuse("from %q is the language being written", from)
	}
	m.editMu.Lock()
	defer m.editMu.Unlock()
	srcEntry, err := m.text(from, key)
	if err != nil {
		return err
	}
	src, ok := fieldValue(srcEntry, field)
	if !ok || strings.TrimSpace(src.V) == "" {
		return refuse("%s.%s: no text in %s to translate from", key, field, from)
	}
	entry, err := m.text(lang, key)
	if err != nil {
		return err
	}
	entry.set(field, translatedValue(value, from, src.V))
	b, _ := entry.MarshalJSON()
	_, err = m.Apply([]Op{{Kind: "text", ID: key, Lang: lang, Value: b}}, author)
	return err
}

// MarkTranslated turns existing authored texts in lang into translations of the
// same texts in from: the value stays, the origin becomes translated with from
// and fromHash of the source as it is now, at = now. keys limit it to those text
// keys; without them every key and field that has a text in both languages and
// is authored in lang is marked. A field already a translation, or whose source
// is itself a translation of lang, is left alone. One batch, all or nothing;
// nothing to mark is refused.
func (m *Model) MarkTranslated(lang, from string, keys []string, author string) ([]TextMark, error) {
	if lang == "" || from == "" || lang == from {
		return nil, refuse("lang and from are two different languages")
	}
	m.editMu.Lock()
	defer m.editMu.Unlock()
	explicit := len(keys) > 0
	if !explicit {
		var err error
		if keys, err = m.textKeys(lang); err != nil {
			return nil, err
		}
	}
	var ops []Op
	var marks []TextMark
	for _, key := range keys {
		dst, err := m.text(lang, key)
		if err != nil {
			return nil, err
		}
		srcEntry, err := m.text(from, key)
		if err != nil {
			return nil, err
		}
		changed := false
		for _, field := range TextFields {
			cur, ok := fieldValue(dst, field)
			if !ok || cur.Origin != "authored" {
				continue
			}
			src, ok := fieldValue(srcEntry, field)
			if !ok || (src.Origin == "translated" && src.From == lang) {
				continue
			}
			dst.set(field, translatedValue(cur.V, from, src.V))
			marks = append(marks, TextMark{key, field})
			changed = true
		}
		if !changed {
			if explicit {
				return nil, refuse("%s: nothing to mark: no authored text in %s with a source in %s", key, lang, from)
			}
			continue
		}
		b, _ := dst.MarshalJSON()
		ops = append(ops, Op{Kind: "text", ID: key, Lang: lang, Value: b})
	}
	if len(ops) == 0 {
		return nil, refuse("nothing to mark: no authored text in %s has a source in %s", lang, from)
	}
	if _, err := m.Apply(ops, author); err != nil {
		return nil, err
	}
	return marks, nil
}

// textKeys are the keys of the text catalogue of lang, in file order.
func (m *Model) textKeys(lang string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, err := m.loadText(lang)
	if err != nil {
		return nil, err
	}
	entries, err := child(d, "entries")
	if err != nil {
		return nil, err
	}
	return append([]string(nil), entries.keys...), nil
}

// SetViewAxis sets the `axis` of a view (CONTRACT §8.1). Like create_view, it
// asks only for a non-empty axis name; the view must exist.
func (m *Model) SetViewAxis(view, axis, author string) error {
	axis = strings.TrimSpace(axis)
	if axis == "" {
		return refuse("axis is empty")
	}
	m.editMu.Lock()
	defer m.editMu.Unlock()
	if _, err := m.view(view); err != nil {
		return err
	}
	v := newObject()
	v.set("id", view)
	v.set("axis", axis)
	b, _ := v.MarshalJSON()
	_, err := m.Apply([]Op{{Kind: "view", ID: view, View: view, Value: b}}, author)
	return err
}
