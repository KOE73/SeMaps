// Package store imports model (depends) and covers the interface rows.
package store

import (
	"io"

	"example.com/sample/model"
)

// Reader is satisfied by MemStore's value method set.
type Reader interface {
	Read() model.Item
}

// Writer is satisfied only by *MemStore.
type Writer interface {
	Write(item model.Item) error
}

// ReadWriter embeds both: extends, native embed.
type ReadWriter interface {
	Reader
	Writer
}

// Source is a defined interface over Reader: extends, native underlying.
type Source Reader

// Any is empty: no implements edges to it.
type Any interface{}

// MemStore implements Reader (methodset) and Writer, ReadWriter (methodset.ptr).
type MemStore struct {
	items []model.Item
	boxed model.Box[model.Item]
}

// Read has a value receiver.
func (m MemStore) Read() model.Item { return m.items[0] }

// Write has a pointer receiver.
func (m *MemStore) Write(item model.Item) error {
	m.items = append(m.items, item)
	return nil
}

// Clone returns its own type: no self edge.
func (m *MemStore) Clone() *MemStore { return m }

// NewMemStore is a plain function, not a constructor: uses, memberKind parameter/return.
func NewMemStore(seed []model.Item, extra ...model.Item) *MemStore {
	return &MemStore{items: append(seed, extra...)}
}

// Split has two results.
func Split(u model.User) (model.Batch, model.Status) { return nil, 0 }

func init() {}

// Log implements io.Writer through *Log (external: methodset.ptr).
type Log struct {
	lines []string
}

// Write has a pointer receiver.
func (l *Log) Write(p []byte) (int, error) {
	l.lines = append(l.lines, string(p))
	return len(p), nil
}

// Flush takes an io.Writer, which puts io among the loaded imports.
func (l *Log) Flush(w io.Writer) error { return nil }

// NotFound implements the predeclared error (external: methodset).
type NotFound struct{}

// Error has a value receiver.
func (NotFound) Error() string { return "not found" }
