package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// extractor holds one run. Symbols are keyed by their id string, not by
// types.Object: separately loaded modules have separate type universes.
type extractor struct {
	o       *options
	rootAbs string
	filter  *pathFilter
	fset    *token.FileSet
	facts   *Facts

	ids      map[string]bool // every symbol id of the output
	kept     map[string]bool // absolute file names that are part of the output
	typeDecl []typeDecl
	funcDecl []funcDecl
	valDecl  []valDecl
	pkgDecl  []pkgDecl
	enumVals map[string][]Member // type id → its constants, in declaration order
	symIndex map[string]int      // id → index in facts.Symbols
}

type pkgDecl struct {
	pkg   *packages.Package
	group int
	files []*ast.File // kept files, sorted by path
}

type typeDecl struct {
	pkg   *packages.Package
	group int
	spec  *ast.TypeSpec
	obj   *types.TypeName
	id    string
}

type funcDecl struct {
	pkg  *packages.Package
	decl *ast.FuncDecl
	obj  *types.Func
}

type valDecl struct {
	pkg  *packages.Package
	spec *ast.ValueSpec
	name *ast.Ident
	obj  types.Object
	id   string
}

func extract(o *options, stderr io.Writer) (*Facts, error) {
	rootAbs, err := filepath.Abs(o.root)
	if err != nil {
		return nil, err
	}
	if r, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = r
	}
	x := &extractor{
		o: o, rootAbs: rootAbs, filter: newPathFilter(o.includes, o.excludes),
		fset: token.NewFileSet(), ids: map[string]bool{}, kept: map[string]bool{},
		enumVals: map[string][]Member{}, symIndex: map[string]int{},
		facts: &Facts{Language: "go", Root: filepath.ToSlash(o.root)},
	}
	groups, err := load(x.fset, rootAbs, o.includes)
	if err != nil {
		return nil, err
	}
	for gi, g := range groups {
		for _, p := range g {
			x.collect(p, gi)
		}
	}
	x.members()
	x.containsAndDepends()
	x.typeEdges()
	x.funcEdges()
	x.valueEdges()
	x.implementsEdges()
	x.external(stderr)

	kinds := []string{"contains", "depends", "extends", "implements"}
	for _, k := range []string{"holds", "uses", "injects"} {
		if o.edges[k] {
			kinds = append(kinds, k)
		}
	}
	sort.Strings(kinds)
	x.facts.EdgeKinds = kinds
	x.facts.normalize()
	return x.facts, nil
}

// rel is a file name relative to root with forward slashes; "" when outside.
func (x *extractor) rel(abs string) string {
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		abs = r
	}
	r, err := filepath.Rel(x.rootAbs, abs)
	if err != nil {
		return ""
	}
	r = filepath.ToSlash(r)
	if r == ".." || strings.HasPrefix(r, "../") {
		return ""
	}
	return r
}

func (x *extractor) pos(p token.Pos) (string, int) {
	pp := x.fset.Position(p)
	return x.rel(pp.Filename), pp.Line
}

func visibility(name string) string {
	if token.IsExported(name) {
		return "exported"
	}
	return "unexported"
}

// modifiers is the access word core understands (core/sync.go isPublicMember):
// exported → public, unexported → private. See docs/extractors/go.md.
func modifiers(name string) []string {
	if token.IsExported(name) {
		return []string{"public"}
	}
	return []string{"private"}
}

func (x *extractor) add(s Symbol) {
	x.ids[s.ID] = true
	x.symIndex[s.ID] = len(x.facts.Symbols)
	x.facts.Symbols = append(x.facts.Symbols, s)
}

func (x *extractor) addEdge(e Edge) {
	if e.From == e.To || !x.ids[e.From] || !x.ids[e.To] {
		return
	}
	x.facts.Edges = append(x.facts.Edges, e)
}

// collect makes the package symbol and every top-level symbol of the kept files.
func (x *extractor) collect(p *packages.Package, group int) {
	type fileAt struct {
		rel  string
		file *ast.File
	}
	var files []fileAt
	for _, f := range p.Syntax {
		abs := x.fset.Position(f.Package).Filename
		r := x.rel(abs)
		if !x.filter.keepFile(r, f) {
			continue
		}
		x.kept[abs] = true
		files = append(files, fileAt{r, f})
	}
	if len(files) == 0 {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	pd := pkgDecl{pkg: p, group: group}
	for _, f := range files {
		pd.files = append(pd.files, f.file)
	}
	x.pkgDecl = append(x.pkgDecl, pd)
	_, line := x.pos(files[0].file.Package)
	x.add(Symbol{ID: p.PkgPath, Kind: "module", NativeKind: "package", Name: p.Name, File: files[0].rel, Line: line})

	for _, f := range files {
		for _, d := range f.file.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, sp := range d.Specs {
					switch sp := sp.(type) {
					case *ast.TypeSpec:
						x.collectType(p, group, sp)
					case *ast.ValueSpec:
						x.collectValues(p, d.Tok, sp)
					}
				}
			case *ast.FuncDecl:
				obj, _ := p.TypesInfo.Defs[d.Name].(*types.Func)
				if obj == nil {
					continue
				}
				x.funcDecl = append(x.funcDecl, funcDecl{p, d, obj})
				if d.Recv != nil || d.Name.Name == "init" || d.Name.Name == "_" {
					continue // methods are members; init is not addressable
				}
				file, line := x.pos(d.Name.Pos())
				x.add(Symbol{ID: p.PkgPath + "." + d.Name.Name, Kind: "function", NativeKind: "function",
					Name: d.Name.Name, Namespace: p.PkgPath, File: file, Line: line, Visibility: visibility(d.Name.Name)})
			}
		}
	}
}

// typeKinds is the kind/nativeKind table of docs/extractors/go.md.
func typeKinds(tn *types.TypeName) (kind, native string) {
	if tn.IsAlias() {
		if types.IsInterface(tn.Type()) {
			return "interface", "alias"
		}
		return "type", "alias"
	}
	switch u := tn.Type().Underlying().(type) {
	case *types.Struct:
		return "type", "struct"
	case *types.Interface:
		return "interface", "interface"
	case *types.Signature:
		return "type", "func"
	case *types.Map:
		return "type", "map"
	case *types.Slice:
		return "type", "slice"
	case *types.Array:
		return "type", "array"
	case *types.Chan:
		return "type", "chan"
	case *types.Pointer:
		return "type", "pointer"
	case *types.Basic:
		return "type", u.Name()
	}
	return "type", "type"
}

func (x *extractor) collectType(p *packages.Package, group int, sp *ast.TypeSpec) {
	tn, _ := p.TypesInfo.Defs[sp.Name].(*types.TypeName)
	if tn == nil || sp.Name.Name == "_" {
		return
	}
	id := p.PkgPath + "." + sp.Name.Name
	kind, native := typeKinds(tn)
	file, line := x.pos(sp.Name.Pos())
	x.add(Symbol{ID: id, Kind: kind, NativeKind: native, Name: sp.Name.Name, Namespace: p.PkgPath,
		File: file, Line: line, Visibility: visibility(sp.Name.Name)})
	x.typeDecl = append(x.typeDecl, typeDecl{p, group, sp, tn, id})
}

func (x *extractor) collectValues(p *packages.Package, tok token.Token, sp *ast.ValueSpec) {
	native := "var"
	if tok == token.CONST {
		native = "const"
	}
	for _, n := range sp.Names {
		obj := p.TypesInfo.Defs[n]
		if obj == nil || n.Name == "_" {
			continue
		}
		id := p.PkgPath + "." + n.Name
		file, line := x.pos(n.Pos())
		x.add(Symbol{ID: id, Kind: "value", NativeKind: native, Name: n.Name, Namespace: p.PkgPath,
			File: file, Line: line, Visibility: visibility(n.Name)})
		x.valDecl = append(x.valDecl, valDecl{p, sp, n, obj, id})
		// A constant of a defined type of the same package is also a member
		// of that type: Go's enum (EXTRACTOR.md §3 "values of enum").
		if c, ok := obj.(*types.Const); ok {
			if named, ok := types.Unalias(c.Type()).(*types.Named); ok && named.Obj().Pkg() == p.Types {
				tid := p.PkgPath + "." + named.Obj().Name()
				x.enumVals[tid] = append(x.enumVals[tid], Member{Kind: "value", Name: n.Name,
					Type: c.Val().ExactString(), Visibility: visibility(n.Name)})
			}
		}
	}
}

func qualifier(p *types.Package) types.Qualifier {
	return func(other *types.Package) string {
		if other == p {
			return ""
		}
		return other.Name()
	}
}

// members fills members of type and interface symbols: fields, declared
// methods of both receivers (a method in a skipped file is left out), and the
// constants of the type.
func (x *extractor) members() {
	for _, td := range x.typeDecl {
		s := &x.facts.Symbols[x.symIndex[td.id]]
		if td.obj.IsAlias() {
			continue // an alias has no members of its own
		}
		q := qualifier(td.pkg.Types)
		ms := []Member{}
		named, _ := td.obj.Type().(*types.Named)
		switch u := td.obj.Type().Underlying().(type) {
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				f := u.Field(i)
				m := Member{Kind: "field", Name: f.Name(), Type: types.TypeString(f.Type(), q), Visibility: visibility(f.Name())}
				if f.Embedded() {
					m.Note = "embedded"
				}
				ms = append(ms, m)
			}
		case *types.Interface:
			var methods []Member
			for i := 0; i < u.NumExplicitMethods(); i++ {
				f := u.ExplicitMethod(i)
				methods = append(methods, Member{Kind: "method", Name: f.Name(), Type: types.TypeString(f.Type(), q), Visibility: visibility(f.Name())})
			}
			sort.SliceStable(methods, func(i, j int) bool { return methods[i].Name < methods[j].Name })
			ms = append(ms, methods...)
		}
		if named != nil {
			var methods []Member
			for i := 0; i < named.NumMethods(); i++ {
				f := named.Method(i)
				if !x.kept[x.fset.Position(f.Pos()).Filename] {
					continue
				}
				m := Member{Kind: "method", Name: f.Name(), Type: types.TypeString(f.Type(), q), Visibility: visibility(f.Name())}
				if sig := f.Type().(*types.Signature); sig.Recv() != nil {
					if _, ptr := sig.Recv().Type().(*types.Pointer); ptr {
						m.Note = "pointer receiver"
					}
				}
				methods = append(methods, m)
			}
			sort.SliceStable(methods, func(i, j int) bool { return methods[i].Name < methods[j].Name })
			ms = append(ms, methods...)
		}
		ms = append(ms, x.enumVals[td.id]...)
		s.Members = &ms
	}
}

func (x *extractor) containsAndDepends() {
	for _, pd := range x.pkgDecl {
		from := pd.pkg.PkgPath
		for _, s := range x.facts.Symbols {
			if s.Kind != "module" && s.Namespace == from {
				x.addEdge(Edge{From: from, To: s.ID, Kind: "contains"})
			}
		}
		for _, f := range pd.files {
			for _, imp := range f.Imports {
				if path, err := strconv.Unquote(imp.Path.Value); err == nil {
					x.addEdge(Edge{From: from, To: path, Kind: "depends"})
				}
			}
		}
	}
}

// objID is the output id of a package-level named type, "" otherwise.
func (x *extractor) objID(obj *types.TypeName) string {
	if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return ""
	}
	id := obj.Pkg().Path() + "." + obj.Name()
	if !x.ids[id] {
		return ""
	}
	return id
}

func (x *extractor) exprText(p *packages.Package, e ast.Expr, t types.Type) string {
	if e != nil {
		return types.ExprString(e)
	}
	return types.TypeString(t, qualifier(p.Types))
}

func viaFrom(h hit, member, memberKind, text string, mods []string) *Via {
	v := &Via{Member: member, MemberKind: memberKind, Modifiers: mods, Text: text, Cardinality: h.card, Deferred: h.deferred}
	if len(h.path) > 0 {
		v.Path = h.path
	}
	return v
}

// typeEdges: struct fields (holds), interface embedding (extends), and the
// right-hand side of aliases and other defined types (docs/extractors/go.md).
func (x *extractor) typeEdges() {
	holds, uses := x.o.edges["holds"], x.o.edges["uses"]
	for _, td := range x.typeDecl {
		p, sp, name := td.pkg, td.spec, td.obj.Name()
		rhs := p.TypesInfo.Types[sp.Type].Type
		if td.obj.IsAlias() {
			if uses {
				for _, h := range x.walk(rhs) {
					x.addEdge(Edge{From: td.id, To: h.to, Kind: "uses", Native: "alias",
						Via: viaFrom(h, name, "self", types.ExprString(sp.Type), nil)})
				}
			}
			continue
		}
		switch t := sp.Type.(type) {
		case *ast.StructType:
			if holds {
				x.structHolds(p, td.id, td.obj.Type().Underlying().(*types.Struct), t, nil)
			}
		case *ast.InterfaceType:
			iface := td.obj.Type().Underlying().(*types.Interface)
			for i := 0; i < iface.NumEmbeddeds(); i++ {
				if n, ok := types.Unalias(iface.EmbeddedType(i)).(*types.Named); ok {
					if to := x.objID(n.Origin().Obj()); to != "" && types.IsInterface(n) {
						x.addEdge(Edge{From: td.id, To: to, Kind: "extends", Native: "embed"})
					}
				}
			}
		case *ast.FuncType:
			if uses {
				for _, h := range x.walkSignature(rhs.(*types.Signature), nil, "", false) {
					x.addEdge(Edge{From: td.id, To: h.to, Kind: "uses", Native: "signature",
						Via: viaFrom(h, name, "self", types.ExprString(sp.Type), nil)})
				}
			}
		default:
			if types.IsInterface(td.obj.Type()) {
				// `type Source Reader`: the same contract under a new name.
				if n, ok := types.Unalias(rhs).(*types.Named); ok {
					if to := x.objID(n.Origin().Obj()); to != "" {
						x.addEdge(Edge{From: td.id, To: to, Kind: "extends", Native: "underlying"})
					}
				}
				continue
			}
			if holds {
				for _, h := range x.walk(rhs) {
					x.addEdge(Edge{From: td.id, To: h.to, Kind: "holds", Native: "underlying",
						Via: viaFrom(h, name, "self", types.ExprString(sp.Type), modifiers(name))})
				}
			}
		}
	}
}

// structHolds emits one holds edge per output symbol reached from each field.
// prefix is the path of an anonymous struct this struct lies in.
func (x *extractor) structHolds(p *packages.Package, from string, st *types.Struct, ast_ *ast.StructType, prefix []string) {
	texts := make([]string, st.NumFields())
	if ast_ != nil {
		i := 0
		for _, f := range ast_.Fields.List {
			n := len(f.Names)
			if n == 0 {
				n = 1
			}
			for k := 0; k < n && i < len(texts); k++ {
				texts[i] = types.ExprString(f.Type)
				i++
			}
		}
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		native := "field"
		if f.Embedded() {
			native = "embed"
		}
		text := texts[i]
		if text == "" {
			text = types.TypeString(f.Type(), qualifier(p.Types))
		}
		for _, h := range x.walk(f.Type()) {
			x.addEdge(Edge{From: from, To: h.to, Kind: "holds", Native: native,
				Via: viaFrom(h, f.Name(), "field", text, modifiers(f.Name()))})
		}
	}
}

// funcEdges: parameters and results of top-level functions and methods (uses).
// Go has no constructors: `New*` functions are plain functions, so there are
// never `memberKind: constructor` edges and `injects` covers nothing.
func (x *extractor) funcEdges() {
	if !x.o.edges["uses"] {
		return
	}
	for _, fd := range x.funcDecl {
		d, p := fd.decl, fd.pkg
		if !x.kept[x.fset.Position(d.Pos()).Filename] || d.Name.Name == "init" || d.Name.Name == "_" {
			continue
		}
		sig := fd.obj.Type().(*types.Signature)
		var from, paramPrefix string
		if sig.Recv() != nil {
			rt := sig.Recv().Type()
			if ptr, ok := rt.(*types.Pointer); ok {
				rt = ptr.Elem()
			}
			n, ok := types.Unalias(rt).(*types.Named)
			if !ok {
				continue
			}
			from = x.objID(n.Origin().Obj())
			paramPrefix = d.Name.Name + "."
		} else {
			from = p.PkgPath + "." + d.Name.Name
		}
		if from == "" {
			continue
		}
		emit := func(list *ast.FieldList, tuple *types.Tuple, kind string) {
			texts := make([]string, tuple.Len())
			if list != nil {
				i := 0
				for _, f := range list.List {
					n := max(len(f.Names), 1)
					for k := 0; k < n && i < len(texts); k++ {
						texts[i] = types.ExprString(f.Type)
						i++
					}
				}
			}
			for i := 0; i < tuple.Len(); i++ {
				v := tuple.At(i)
				var member string
				if kind == "parameter" {
					pn := v.Name()
					if pn == "" || pn == "_" {
						pn = "#" + strconv.Itoa(i)
					}
					member = paramPrefix + pn
				} else {
					member = d.Name.Name
					if tuple.Len() > 1 {
						rn := v.Name()
						if rn == "" || rn == "_" {
							rn = "#" + strconv.Itoa(i)
						}
						member += "." + rn
					}
				}
				text := texts[i]
				if text == "" {
					text = types.TypeString(v.Type(), qualifier(p.Types))
				}
				for _, h := range x.walk(v.Type()) {
					x.addEdge(Edge{From: from, To: h.to, Kind: "uses", Via: viaFrom(h, member, kind, text, nil)})
				}
			}
		}
		emit(d.Type.Params, sig.Params(), "parameter")
		emit(d.Type.Results, sig.Results(), "return")
	}
}

// valueEdges: the own type of a const or var (uses, memberKind self).
func (x *extractor) valueEdges() {
	if !x.o.edges["uses"] {
		return
	}
	for _, vd := range x.valDecl {
		text := x.exprText(vd.pkg, vd.spec.Type, vd.obj.Type())
		for _, h := range x.walk(vd.obj.Type()) {
			x.addEdge(Edge{From: vd.id, To: h.to, Kind: "uses", Via: viaFrom(h, vd.name.Name, "self", text, nil)})
		}
	}
}

// implementsEdges: every non-interface defined type against every interface
// of the output, both from the same load group (one type universe). Generic
// types and interfaces, constraint interfaces and empty interfaces are skipped.
func (x *extractor) implementsEdges() {
	type iface struct {
		id    string
		group int
		t     *types.Interface
	}
	var ifaces []iface
	for _, td := range x.typeDecl {
		n, ok := td.obj.Type().(*types.Named)
		if td.obj.IsAlias() || !ok || n.TypeParams().Len() > 0 {
			continue
		}
		if it, ok := n.Underlying().(*types.Interface); ok && it.IsMethodSet() && it.NumMethods() > 0 {
			ifaces = append(ifaces, iface{td.id, td.group, it})
		}
	}
	for _, td := range x.typeDecl {
		n, ok := td.obj.Type().(*types.Named)
		if td.obj.IsAlias() || !ok || n.TypeParams().Len() > 0 || types.IsInterface(n) {
			continue
		}
		for _, it := range ifaces {
			if it.group != td.group {
				continue
			}
			switch {
			case types.Implements(n, it.t):
				x.addEdge(Edge{From: td.id, To: it.id, Kind: "implements", Native: "methodset"})
			case types.Implements(types.NewPointer(n), it.t):
				x.addEdge(Edge{From: td.id, To: it.id, Kind: "implements", Native: "methodset.ptr"})
			}
		}
	}
}

// external handles --implements names (docs/extractors/go.md, "Внешние
// интерфейсы"). A name already in the output is computed like any interface.
// Otherwise it is resolved per load group — through the transitive imports of
// the group's packages, or the universe for `error` — and every non-interface
// defined type of that group is tested against it. The external symbol is
// printed only when at least one implements edge targets it; an unresolvable
// name is a stderr warning.
func (x *extractor) external(stderr io.Writer) {
	groups := map[int][]*types.Package{}
	var order []int
	for _, pd := range x.pkgDecl {
		if _, ok := groups[pd.group]; !ok {
			order = append(order, pd.group)
		}
		groups[pd.group] = append(groups[pd.group], pd.pkg.Types)
	}
	sort.Ints(order)
	for _, name := range x.o.implements {
		if x.ids[name] {
			continue
		}
		var sym *Symbol
		var edges []Edge
		resolved := false
		for _, g := range order {
			obj := lookupExternal(groups[g], name)
			if obj == nil {
				continue
			}
			n, ok := obj.Type().(*types.Named)
			if !ok || n.TypeParams().Len() > 0 {
				continue
			}
			it, ok := n.Underlying().(*types.Interface)
			if !ok || !it.IsMethodSet() || it.NumMethods() == 0 {
				continue
			}
			resolved = true
			if sym == nil {
				sym = externalSymbol(name, obj, it)
			}
			for _, td := range x.typeDecl {
				tn, ok := td.obj.Type().(*types.Named)
				if td.group != g || td.obj.IsAlias() || !ok || tn.TypeParams().Len() > 0 || types.IsInterface(tn) {
					continue
				}
				switch {
				case types.Implements(tn, it):
					edges = append(edges, Edge{From: td.id, To: name, Kind: "implements", Native: "methodset"})
				case types.Implements(types.NewPointer(tn), it):
					edges = append(edges, Edge{From: td.id, To: name, Kind: "implements", Native: "methodset.ptr"})
				}
			}
		}
		if !resolved {
			fmt.Fprintf(stderr, "--implements %s: not resolved to a non-empty, non-generic interface in the loaded packages; skipped\n", name)
			continue
		}
		if len(edges) == 0 {
			continue // nothing implements it: no symbol, no noise
		}
		x.add(*sym)
		for _, e := range edges {
			x.addEdge(e)
		}
	}
}

// lookupExternal finds `<importpath>.<Name>` among pkgs and their transitive
// imports, or the predeclared `error`.
func lookupExternal(pkgs []*types.Package, name string) types.Object {
	if name == "error" {
		return types.Universe.Lookup("error")
	}
	dot := strings.LastIndex(name, ".")
	if dot <= 0 || dot == len(name)-1 || strings.LastIndex(name, "/") > dot {
		return nil
	}
	path, local := name[:dot], name[dot+1:]
	seen := map[*types.Package]bool{}
	var walk func(p *types.Package) types.Object
	walk = func(p *types.Package) types.Object {
		if p == nil || seen[p] {
			return nil
		}
		seen[p] = true
		if p.Path() == path {
			if o, ok := p.Scope().Lookup(local).(*types.TypeName); ok && o.Exported() {
				return o
			}
			return nil
		}
		for _, imp := range p.Imports() {
			if o := walk(imp); o != nil {
				return o
			}
		}
		return nil
	}
	for _, p := range pkgs {
		if o := walk(p); o != nil {
			return o
		}
	}
	return nil
}

// externalSymbol: no file, line or visibility; members are the interface's
// full method set (cheap, and it shows what implementing means).
func externalSymbol(id string, obj types.Object, it *types.Interface) *Symbol {
	s := &Symbol{ID: id, Kind: "interface", NativeKind: "external", Name: obj.Name()}
	if obj.Pkg() != nil {
		s.Namespace = obj.Pkg().Path()
	}
	q := qualifier(obj.Pkg())
	ms := []Member{}
	for i := 0; i < it.NumMethods(); i++ {
		f := it.Method(i)
		ms = append(ms, Member{Kind: "method", Name: f.Name(), Type: types.TypeString(f.Type(), q), Visibility: visibility(f.Name())})
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })
	s.Members = &ms
	return s
}
