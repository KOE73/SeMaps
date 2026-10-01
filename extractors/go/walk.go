package main

import (
	"go/types"
	"strconv"
)

// hit is one output symbol reached inside a member type, with the
// member-relation features of docs/extractors/go.md "Member-relation features".
type hit struct {
	to       string
	path     []string
	card     string
	deferred bool
}

// walk finds every output symbol a type mentions. It stops at a named type of
// the output (its insides are its own business) and looks only at type
// arguments of any other named type (a library type is opaque).
func (x *extractor) walk(t types.Type) []hit {
	var out []hit
	x.walkInto(t, nil, "", false, &out)
	return out
}

func appendPath(path []string, seg string) []string {
	p := make([]string, len(path), len(path)+1)
	copy(p, path)
	return append(p, seg)
}

func (x *extractor) walkInto(t types.Type, path []string, card string, deferred bool, out *[]hit) {
	switch t := t.(type) {
	case *types.Alias:
		if id := x.objID(t.Obj()); id != "" {
			x.hit(id, path, card, deferred, out)
			return
		}
		x.walkInto(types.Unalias(t), path, card, deferred, out)
	case *types.Named:
		if id := x.objID(t.Origin().Obj()); id != "" {
			x.hit(id, path, card, deferred, out)
		}
		args := t.TypeArgs()
		for i := 0; i < args.Len(); i++ {
			x.walkInto(args.At(i), appendPath(path, "arg:"+strconv.Itoa(i)), card, deferred, out)
		}
	case *types.Pointer:
		// A pointer is how Go shares an object, not how it says "maybe":
		// transparent, no path segment, cardinality unchanged.
		x.walkInto(t.Elem(), path, card, deferred, out)
	case *types.Slice:
		x.walkInto(t.Elem(), appendPath(path, "item"), or(card, "many"), deferred, out)
	case *types.Array:
		x.walkInto(t.Elem(), appendPath(path, "item"), or(card, "many"), deferred, out)
	case *types.Map:
		c := or(card, "keyed")
		x.walkInto(t.Key(), appendPath(path, "key"), c, deferred, out)
		x.walkInto(t.Elem(), appendPath(path, "value"), c, deferred, out)
	case *types.Chan:
		x.walkInto(t.Elem(), appendPath(path, "item"), or(card, "many"), true, out)
	case *types.Signature:
		*out = append(*out, x.walkSignature(t, path, card, true)...)
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			f := t.Field(i)
			x.walkInto(f.Type(), appendPath(path, "field:"+f.Name()), card, deferred, out)
		}
	}
	// *types.Basic, *types.Interface literals, *types.TypeParam, tuples: nothing.
}

// walkSignature: parameters under `arg:i`, results under `result` (one) or
// `result:i` (several). deferred marks a function value held or passed: the
// object comes when it is called.
func (x *extractor) walkSignature(s *types.Signature, path []string, card string, deferred bool) []hit {
	var out []hit
	for i := 0; i < s.Params().Len(); i++ {
		x.walkInto(s.Params().At(i).Type(), appendPath(path, "arg:"+strconv.Itoa(i)), card, deferred, &out)
	}
	n := s.Results().Len()
	for i := 0; i < n; i++ {
		seg := "result"
		if n > 1 {
			seg = "result:" + strconv.Itoa(i)
		}
		x.walkInto(s.Results().At(i).Type(), appendPath(path, seg), card, deferred, &out)
	}
	return out
}

func (x *extractor) hit(id string, path []string, card string, deferred bool, out *[]hit) {
	*out = append(*out, hit{to: id, path: path, card: or(card, "one"), deferred: deferred})
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
