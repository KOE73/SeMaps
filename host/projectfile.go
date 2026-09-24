package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProjectExt marks a project file: `<name>.semaps` in the project root. The
// directory holding it is the project root; every path inside is relative to it.
// Opening the file (double click, Enter in a file manager, or `semaps x.semaps`)
// opens the project.
const ProjectExt = ".semaps"

// project is the content of a .semaps file. Every key is optional.
type project struct {
	File       string
	Root       string          // the folder holding the file; every path is relative to it
	Name       string          // shown in the console
	Workspace  string          // default: docs/diagrams
	SourceRoot string          // default: the project root
	Port       int             // default: 8777
	Extractors []extractorConf // ADR_20260924-3 §2
}

// extractorConf is one entry of `extractors:` — what to extract and into which
// model project. Paths stay as written, relative to the project root.
type extractorConf struct {
	ID       string   `yaml:"id"`
	Language string   `yaml:"language"`
	Project  string   `yaml:"project"`
	Root     string   `yaml:"root,omitempty"`
	Include  []string `yaml:"include,omitempty"`
	Exclude  []string `yaml:"exclude,omitempty"`
	// Command replaces the found extractor. Only ever written by hand in the
	// file: nothing from outside sets it (ADR_20260924-3 §5).
	Command string `yaml:"command,omitempty"`
}

type projectFile struct {
	Version    int             `yaml:"version"`
	Name       string          `yaml:"name"`
	Workspace  string          `yaml:"workspace"`
	SourceRoot string          `yaml:"source_root"`
	Port       int             `yaml:"port"`
	Extractors []extractorConf `yaml:"extractors"`
}

// loadProject reads a .semaps file. It is YAML; the flat `key: value` files
// written before extractors existed are valid YAML and read as they were.
func loadProject(file string) (project, error) {
	root := filepath.Dir(file)
	p := project{File: file, Root: root}
	data, err := os.ReadFile(file)
	if err != nil {
		return p, err
	}
	var f projectFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return p, fmt.Errorf("%s: %v", file, err)
	}
	if f.Workspace == "" {
		f.Workspace = "docs/diagrams"
	}
	if f.SourceRoot == "" {
		f.SourceRoot = "."
	}
	if err := validateExtractors(f.Extractors); err != nil {
		return p, fmt.Errorf("%s: %v", file, err)
	}
	p.Name, p.Port, p.Extractors = f.Name, f.Port, f.Extractors
	p.Workspace = filepath.Join(root, filepath.FromSlash(f.Workspace))
	p.SourceRoot = filepath.Join(root, filepath.FromSlash(f.SourceRoot))
	return p, nil
}

func validateExtractors(list []extractorConf) error {
	var problems []string
	seen := map[string]bool{}
	for i, e := range list {
		where := fmt.Sprintf("extractors[%d]", i)
		if e.ID == "" {
			problems = append(problems, where+": id is required")
		} else if seen[e.ID] {
			problems = append(problems, fmt.Sprintf("%s: id %q repeats", where, e.ID))
		}
		seen[e.ID] = true
		if e.Language == "" {
			problems = append(problems, where+": language is required")
		}
		if e.Project == "" {
			problems = append(problems, where+": project is required")
		}
		for _, path := range append([]string{e.Root}, e.Include...) {
			if path != "" && (filepath.IsAbs(filepath.FromSlash(path)) || filepath.VolumeName(filepath.FromSlash(path)) != "") {
				problems = append(problems, fmt.Sprintf("%s: %q must be relative to the project root", where, path))
			}
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// extractorPatch is what may change in an entry from outside the file: data,
// never the command line (ADR_20260924-3 §5). Nil leaves a field as it is.
type extractorPatch struct {
	Language *string
	Project  *string
	Root     *string
	Include  *[]string
	Exclude  *[]string
}

// patchExtractor changes, or adds, one entry of `extractors:`. The file is
// edited as a YAML tree, so comments, key order and untouched entries survive.
func patchExtractor(file, id string, patch extractorPatch) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %v", file, err)
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	top := doc.Content[0]
	if top.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: expected a mapping at the top", file)
	}
	list := mapValue(top, "extractors")
	if list == nil || list.Kind != yaml.SequenceNode {
		list = &yaml.Node{Kind: yaml.SequenceNode}
		setMapValue(top, "extractors", list)
	}
	var entry *yaml.Node
	for _, item := range list.Content {
		if v := mapValue(item, "id"); v != nil && v.Value == id {
			entry = item
		}
	}
	if entry == nil {
		entry = &yaml.Node{Kind: yaml.MappingNode}
		setMapValue(entry, "id", scalar(id))
		list.Content = append(list.Content, entry)
	}
	setString := func(key string, v *string) {
		if v != nil {
			setMapValue(entry, key, scalar(*v))
		}
	}
	setList := func(key string, v *[]string) {
		if v == nil {
			return
		}
		seq := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, s := range *v {
			seq.Content = append(seq.Content, scalar(s))
		}
		setMapValue(entry, key, seq)
	}
	setString("language", patch.Language)
	setString("project", patch.Project)
	setString("root", patch.Root)
	setList("include", patch.Include)
	setList("exclude", patch.Exclude)

	// Check the result as loadProject would before touching the file.
	var check projectFile
	if err := doc.Decode(&check); err != nil {
		return err
	}
	if err := validateExtractors(check.Extractors); err != nil {
		return err
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(file, out.Bytes(), 0o644)
}

func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: v} }

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMapValue replaces a value, keeping the comments attached to the old one,
// or appends the key at the end.
func setMapValue(m *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			old := m.Content[i+1]
			v.HeadComment, v.LineComment, v.FootComment = old.HeadComment, old.LineComment, old.FootComment
			m.Content[i+1] = v
			return
		}
	}
	m.Content = append(m.Content, scalar(key), v)
}
