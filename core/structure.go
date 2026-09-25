package core

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func renameObjectKey(o *object, old, next string) {
	value, ok := o.vals[old]
	if !ok {
		return
	}
	delete(o.vals, old)
	o.vals[next] = value
	for i, key := range o.keys {
		if key == old {
			o.keys[i] = next
			break
		}
	}
}

// RenameProject is a clean-state structural operation. The host checks and
// serializes against the live model before calling it.
func RenameProject(workspace, oldID, newID string) error {
	if oldID == newID {
		return nil
	}
	if oldID == "" || newID == "" || strings.ContainsAny(oldID, `/\`) || strings.ContainsAny(newID, `/\`) || oldID == ".." || newID == ".." {
		return refuse("invalid project id")
	}
	oldDir := filepath.Join(workspace, "projects", oldID)
	newDir := filepath.Join(workspace, "projects", newID)
	if _, err := os.Stat(newDir); err == nil {
		return fs.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	manifest, err := loadDoc(filepath.Join(oldDir, "project.json"))
	if err != nil {
		return err
	}
	if manifest == nil {
		return fs.ErrNotExist
	}
	views, _ := filepath.Glob(filepath.Join(oldDir, "views", "*.view.json"))
	viewDocs := map[string]*object{}
	for _, file := range views {
		doc, err := loadDoc(file)
		if err != nil {
			return err
		}
		viewDocs[filepath.Base(file)] = doc
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return err
	}
	manifest.set("id", newID)
	if err := saveDoc(filepath.Join(newDir, "project.json"), manifest); err != nil {
		return err
	}
	for file, doc := range viewDocs {
		doc.set("project", newID)
		if err := saveDoc(filepath.Join(newDir, "views", file), doc); err != nil {
			return err
		}
	}
	return nil
}

// RenameView moves one clean view and its text key without changing the
// position of that key in any text catalogue.
func RenameView(workspace, project, oldID, newID string) error {
	if oldID == newID {
		return nil
	}
	if oldID == "" || newID == "" || strings.ContainsAny(oldID, `/\`) || strings.ContainsAny(newID, `/\`) || oldID == ".." || newID == ".." {
		return refuse("invalid view id")
	}
	dir := filepath.Join(workspace, "projects", project)
	oldFile, view, err := loadView(dir, oldID)
	if err != nil {
		return err
	}
	newFile := filepath.Join(dir, "views", newID+".view.json")
	if _, err := os.Stat(newFile); err == nil {
		return fs.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	manifest, err := loadDoc(filepath.Join(dir, "project.json"))
	if err != nil {
		return err
	}
	texts, _ := filepath.Glob(filepath.Join(dir, "text.*.json"))
	textDocs := map[string]*object{}
	for _, file := range texts {
		doc, err := loadDoc(file)
		if err != nil {
			return err
		}
		textDocs[file] = doc
	}
	if err := os.Rename(oldFile, newFile); err != nil {
		return err
	}
	view.set("id", newID)
	if err := saveDoc(newFile, view); err != nil {
		return err
	}
	if manifest != nil && manifest.str("defaultView") == oldID {
		manifest.set("defaultView", newID)
		if err := saveDoc(filepath.Join(dir, "project.json"), manifest); err != nil {
			return err
		}
	}
	for file, doc := range textDocs {
		entries, err := child(doc, "entries")
		if err != nil {
			return err
		}
		if _, ok := entries.vals[oldID]; ok {
			renameObjectKey(entries, oldID, newID)
			doc.set("entries", entries)
			if err := saveDoc(file, doc); err != nil {
				return err
			}
		}
	}
	return nil
}
