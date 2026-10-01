package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// pathFilter decides which files are part of the output: --include prefixes,
// the Go defaults (docs/extractors/go.md "What is skipped") and --exclude globs.
type pathFilter struct {
	includes []string
	excludes []*regexp.Regexp
}

func normRel(p string) string { return strings.Trim(filepath.ToSlash(p), "/") }

func newPathFilter(includes, excludes []string) *pathFilter {
	f := &pathFilter{}
	for _, i := range includes {
		if n := normRel(i); n != "" && n != "." {
			f.includes = append(f.includes, strings.TrimPrefix(n, "./"))
		}
	}
	for _, g := range excludes {
		f.excludes = append(f.excludes, globToRegexp(g))
	}
	return f
}

// globToRegexp: `**` any run of characters, `*` within one segment, `?` one character.
func globToRegexp(glob string) *regexp.Regexp {
	q := regexp.QuoteMeta(normRel(glob))
	q = strings.ReplaceAll(q, `\*\*`, "\x00")
	q = strings.ReplaceAll(q, `\*`, "[^/]*")
	q = strings.ReplaceAll(q, "\x00", ".*")
	q = strings.ReplaceAll(q, `\?`, ".")
	return regexp.MustCompile("^" + q + "$")
}

func (f *pathFilter) included(rel string) bool {
	if len(f.includes) == 0 {
		return true
	}
	for _, p := range f.includes {
		if rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

// excludedByPath: the Go defaults on the path plus --exclude. Generated files
// are recognised by their header, see keepFile.
func (f *pathFilter) excludedByPath(rel string) bool {
	segs := strings.Split(rel, "/")
	for _, s := range segs[:len(segs)-1] {
		if s == "vendor" || s == "testdata" {
			return true
		}
	}
	if strings.HasSuffix(segs[len(segs)-1], "_test.go") {
		return true
	}
	for _, re := range f.excludes {
		if re.MatchString(rel) {
			return true
		}
	}
	return false
}

// keepFile: the file lies under root, passes the filter and is not generated
// (`// Code generated ... DO NOT EDIT.`, ast.IsGenerated).
func (f *pathFilter) keepFile(rel string, file *ast.File) bool {
	if rel == "" || strings.HasPrefix(rel, "../") || rel == ".." {
		return false
	}
	return f.included(rel) && !f.excludedByPath(rel) && !ast.IsGenerated(file)
}

const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo

// moduleRoot is the nearest directory at or above dir holding a go.mod; dir
// itself when there is none (the go command then says what is wrong).
func moduleRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		up := filepath.Dir(d)
		if up == d {
			return dir
		}
		d = up
	}
}

// load reads every package under root (or under each --include) with the go
// command. Includes are grouped by the module they lie in, one packages.Load
// per module, so types of one module share one type universe (implements
// needs that) and an --include may point into a nested module. Groups come
// back sorted by module directory, packages by import path. Any package error
// fails the run: partial facts would mark half the registry missing
// (EXTRACTOR.md §1).
func load(fset *token.FileSet, rootAbs string, includes []string) ([][]*packages.Package, error) {
	dirs := []string{rootAbs}
	if len(includes) > 0 {
		dirs = nil
		for _, inc := range includes {
			d := filepath.Join(rootAbs, filepath.FromSlash(inc))
			st, err := os.Stat(d)
			if err != nil {
				return nil, fmt.Errorf("--include %s: %v", inc, err)
			}
			if !st.IsDir() {
				d = filepath.Dir(d)
			}
			dirs = append(dirs, d)
		}
	}
	patterns := map[string][]string{}
	for _, d := range dirs {
		m := moduleRoot(d)
		rel, err := filepath.Rel(m, d)
		if err != nil {
			return nil, err
		}
		pat := "./..."
		if rel = filepath.ToSlash(rel); rel != "." {
			pat = "./" + rel + "/..."
		}
		patterns[m] = append(patterns[m], pat)
	}
	mods := make([]string, 0, len(patterns))
	for m := range patterns {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	seen := map[string]bool{}
	var groups [][]*packages.Package
	for _, m := range mods {
		cfg := &packages.Config{Mode: loadMode, Dir: m, Fset: fset, Tests: false}
		pkgs, err := packages.Load(cfg, patterns[m]...)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %v", m, err)
		}
		var errs []string
		for _, p := range pkgs {
			for _, e := range p.Errors {
				errs = append(errs, e.Error())
			}
		}
		if len(errs) > 0 {
			sort.Strings(errs)
			return nil, fmt.Errorf("loading %s:\n  %s", m, strings.Join(errs, "\n  "))
		}
		var g []*packages.Package
		for _, p := range pkgs {
			if !seen[p.PkgPath] && p.Types != nil {
				seen[p.PkgPath] = true
				g = append(g, p)
			}
		}
		sort.Slice(g, func(i, j int) bool { return g[i].PkgPath < g[j].PkgPath })
		groups = append(groups, g)
	}
	return groups, nil
}
