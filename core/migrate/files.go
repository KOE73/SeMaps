package migrate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const targetVersion = 5

// fileOut is a computed change of one file on disk.
type fileOut struct {
	rel    string
	path   string
	data   []byte
	remove bool
}

// ferr builds an error that names the file and the field.
func ferr(file, field, format string, a ...any) error {
	msg := fmt.Sprintf(format, a...)
	if field == "" {
		return fmt.Errorf("%s: %s", file, msg)
	}
	return fmt.Errorf("%s: %s: %s", file, field, msg)
}

// file is one JSON document of the workspace, edited in memory.
type file struct {
	rel     string // slash-separated path relative to the workspace
	path    string
	orig    []byte // nil for a file that does not exist yet
	root    *obj
	dirty   bool // structurally changed: re-serialize
	remove  bool
	created bool
	// bump: set contractVersion to the target. ensureVersion adds the key when
	// the file has none; otherwise a missing key is left missing.
	bump          bool
	ensureVersion bool
}

func loadFile(ws, rel string, required bool) (*file, error) {
	path := filepath.Join(ws, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && !required {
			return nil, nil
		}
		return nil, ferr(rel, "", "не читается: %v", err)
	}
	t, err := parseTree(data)
	if err != nil {
		return nil, ferr(rel, "", "неверный JSON: %v", err)
	}
	root, ok := t.(*obj)
	if !ok {
		return nil, ferr(rel, "", "ожидался JSON-объект верхнего уровня")
	}
	return &file{rel: rel, path: path, orig: data, root: root}, nil
}

// newFile makes a file that does not exist on disk yet.
func newFile(ws, rel string) *file {
	root := newObj()
	root.set("contractVersion", jsonInt(targetVersion))
	return &file{rel: rel, path: filepath.Join(ws, filepath.FromSlash(rel)), root: root, created: true}
}

// array returns the array under key, or an error naming the field.
func (f *file) array(key string) ([]any, error) {
	v, ok := f.root.lookup(key)
	if !ok {
		return nil, nil
	}
	a, ok := asArr(v)
	if !ok {
		return nil, ferr(f.rel, key, "ожидался массив")
	}
	return a, nil
}

// output computes the new content of the file. changed is false when the file
// stays byte-identical.
func (f *file) output() (fileOut, bool, error) {
	out := fileOut{rel: f.rel, path: f.path}
	if f.remove {
		out.remove = true
		return out, true, nil
	}
	if f.bump && f.ensureVersion && !f.root.has("contractVersion") {
		f.root.set("contractVersion", jsonInt(targetVersion))
		f.dirty = true
	}
	if f.bump && f.dirty && f.root.has("contractVersion") {
		f.root.set("contractVersion", jsonInt(targetVersion))
	}
	if f.dirty || f.created {
		data, err := encodeTree(f.root)
		if err != nil {
			return out, false, ferr(f.rel, "", "%v", err)
		}
		out.data = data
		return out, f.created || !bytes.Equal(data, f.orig), nil
	}
	if f.bump {
		data, changed, err := bumpVersionText(f.orig, targetVersion)
		if err != nil {
			return out, false, ferr(f.rel, "", "%v", err)
		}
		out.data = data
		return out, changed, nil
	}
	return out, false, nil
}
