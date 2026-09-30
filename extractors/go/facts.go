package main

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// Facts is the document of EXTRACTOR.md §2.
type Facts struct {
	Language  string   `json:"language"`
	Root      string   `json:"root"`
	EdgeKinds []string `json:"edgeKinds"`
	Symbols   []Symbol `json:"symbols"`
	Edges     []Edge   `json:"edges"`
}

type Symbol struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	NativeKind string    `json:"nativeKind"`
	Name       string    `json:"name"`
	Namespace  string    `json:"namespace,omitempty"`
	File       string    `json:"file"`
	Line       int       `json:"line,omitempty"`
	Visibility string    `json:"visibility,omitempty"`
	Members    *[]Member `json:"members,omitempty"` // nil: not given; empty: given and empty
}

type Member struct {
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	Visibility string `json:"visibility,omitempty"`
	Note       string `json:"note,omitempty"`
}

type Edge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Native string `json:"native,omitempty"` // ADR_20260927 §3
	Via    *Via   `json:"via,omitempty"`
}

type Via struct {
	Member      string   `json:"member,omitempty"`
	MemberKind  string   `json:"memberKind,omitempty"`
	Modifiers   []string `json:"modifiers,omitempty"`
	Text        string   `json:"text,omitempty"`
	Path        []string `json:"path,omitempty"`
	Cardinality string   `json:"cardinality,omitempty"`
	Mutability  string   `json:"mutability,omitempty"`
	Deferred    bool     `json:"deferred,omitempty"`
}

// edgeKey is the order and identity of EXTRACTOR.md §3 / core compareEdges:
// (from, to, kind, via.member, via.path joined by ",").
func edgeKey(e Edge) [5]string {
	var member, path string
	if e.Via != nil {
		member, path = e.Via.Member, strings.Join(e.Via.Path, ",")
	}
	return [5]string{e.From, e.To, e.Kind, member, path}
}

func lessKey(a, b [5]string) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// normalize sorts symbols by id and edges by key, dropping repeated keys
// (the first one produced wins; production order is itself deterministic).
func (f *Facts) normalize() {
	sort.SliceStable(f.Symbols, func(i, j int) bool { return f.Symbols[i].ID < f.Symbols[j].ID })
	sort.SliceStable(f.Edges, func(i, j int) bool { return lessKey(edgeKey(f.Edges[i]), edgeKey(f.Edges[j])) })
	out := f.Edges[:0]
	for i, e := range f.Edges {
		if i > 0 && edgeKey(out[len(out)-1]) == edgeKey(e) {
			continue
		}
		out = append(out, e)
	}
	f.Edges = out
	if f.Symbols == nil {
		f.Symbols = []Symbol{}
	}
	if f.Edges == nil {
		f.Edges = []Edge{}
	}
}

func marshal(f *Facts) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // `<-chan T`, `map[K]V` stay as written
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
