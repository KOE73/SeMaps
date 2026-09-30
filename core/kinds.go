package core

// The dictionary, kinds.json (CONTRACT §6, ADR_20260927-6, ADR_20260930-2):
// groups of entity kinds and groups of relation types, each with names,
// descriptions and a base style. The tool ships a default; a workspace
// kinds.json adds to it. The default comes in as bytes: core does not know
// where the tool keeps it.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// KindsFile is the workspace dictionary next to styles.json.
const KindsFile = "kinds.json"

// ContractVersion is the one shape the loader reads (CONTRACT §2).
const ContractVersion = 5

type Kind struct {
	ID          string            `json:"id"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description,omitempty"`
	// Container: an entity of this kind is drawn as a frame and may hold other placements.
	Container bool `json:"container,omitempty"`
	// Style is the kind's base style; empty — the style whose id is the kind (CONTRACT §11.5).
	Style string `json:"style,omitempty"`
}

type KindGroup struct {
	ID          string            `json:"id"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description,omitempty"`
	Kinds       []Kind            `json:"kinds"`
}

// RelationType is a type of relation with its texts and base style. It is the
// dictionary entry; a project's relation-types.json says which of the types the
// project uses, with their origin and visibility (CONTRACT §5).
type RelationType struct {
	ID          string            `json:"id"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description,omitempty"`
	// Style is the type's base style; empty — the style whose id is the type (CONTRACT §11.5).
	Style string `json:"style,omitempty"`
}

type RelationGroup struct {
	ID          string            `json:"id"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description,omitempty"`
	Types       []RelationType    `json:"types"`
}

// KindCatalog is the merged dictionary. A nil catalog knows nothing.
type KindCatalog struct {
	Groups         []KindGroup     `json:"groups"`
	RelationGroups []RelationGroup `json:"relationGroups"`
	index          map[string]Kind
	relIndex       map[string]RelationType
}

type kindsDoc struct {
	ContractVersion *int            `json:"contractVersion"`
	Groups          []KindGroup     `json:"groups"`
	RelationGroups  []RelationGroup `json:"relationGroups"`
}

func parseKindsDoc(name string, data []byte) (*kindsDoc, error) {
	var d kindsDoc
	if len(bytes.TrimSpace(data)) == 0 {
		return &d, nil
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if d.ContractVersion != nil && *d.ContractVersion != ContractVersion {
		return nil, fmt.Errorf("%s: contractVersion %d — нужен %d (CONTRACT §6)", name, *d.ContractVersion, ContractVersion)
	}
	for gi, g := range d.Groups {
		if g.ID == "" {
			return nil, fmt.Errorf("%s: groups[%d]: id is empty (CONTRACT §6)", name, gi)
		}
		if len(g.Name) == 0 {
			return nil, fmt.Errorf("%s: group %s: name is empty (CONTRACT §6)", name, g.ID)
		}
		for ki, k := range g.Kinds {
			if k.ID == "" {
				return nil, fmt.Errorf("%s: group %s: kinds[%d]: id is empty (CONTRACT §6)", name, g.ID, ki)
			}
			if len(k.Name) == 0 {
				return nil, fmt.Errorf("%s: kind %s: name is empty (CONTRACT §6)", name, k.ID)
			}
		}
	}
	for gi, g := range d.RelationGroups {
		if g.ID == "" {
			return nil, fmt.Errorf("%s: relationGroups[%d]: id is empty (CONTRACT §6)", name, gi)
		}
		if len(g.Name) == 0 {
			return nil, fmt.Errorf("%s: relation group %s: name is empty (CONTRACT §6)", name, g.ID)
		}
		for ti, t := range g.Types {
			if t.ID == "" {
				return nil, fmt.Errorf("%s: relation group %s: types[%d]: id is empty (CONTRACT §6)", name, g.ID, ti)
			}
			if len(t.Name) == 0 {
				return nil, fmt.Errorf("%s: relation type %s: name is empty (CONTRACT §6)", name, t.ID)
			}
		}
	}
	return &d, nil
}

// mergeGroups applies the rule of CONTRACT §6 to one kind of group: a
// workspace group with a known id replaces that group entirely, in its place;
// a new group is appended; a workspace entry with a known id replaces the
// earlier entry entirely — the earlier one leaves its group.
func mergeGroups[G any, E any](base, extra []G, gid func(G) string, entries func(G) []E, withEntries func(G, []E) G, eid func(E) string) []G {
	fromWorkspace := map[string]bool{}
	for _, g := range extra {
		for _, e := range entries(g) {
			fromWorkspace[eid(e)] = true
		}
	}
	groups := make([]G, 0, len(base)+len(extra))
	for _, g := range base {
		kept := make([]E, 0, len(entries(g)))
		for _, e := range entries(g) {
			if !fromWorkspace[eid(e)] {
				kept = append(kept, e)
			}
		}
		groups = append(groups, withEntries(g, kept))
	}
	for _, g := range extra {
		replaced := false
		for i := range groups {
			if gid(groups[i]) == gid(g) {
				groups[i], replaced = g, true
				break
			}
		}
		if !replaced {
			groups = append(groups, g)
		}
	}
	return groups
}

// uniqueIDs reports the first id that two groups share: an id is unique
// across the whole catalog.
func uniqueIDs[G any, E any](what, name string, groups []G, gid func(G) string, entries func(G) []E, eid func(E) string) error {
	seen := map[string]string{}
	for _, g := range groups {
		for _, e := range entries(g) {
			if other, ok := seen[eid(e)]; ok {
				return fmt.Errorf("%s: %s %s is in groups %s and %s; an id is unique across the catalog (CONTRACT §6)", name, what, eid(e), other, gid(g))
			}
			seen[eid(e)] = gid(g)
		}
	}
	return nil
}

var (
	kindGroupID     = func(g KindGroup) string { return g.ID }
	kindEntries     = func(g KindGroup) []Kind { return g.Kinds }
	kindWith        = func(g KindGroup, k []Kind) KindGroup { g.Kinds = k; return g }
	kindID          = func(k Kind) string { return k.ID }
	relGroupID      = func(g RelationGroup) string { return g.ID }
	relEntries      = func(g RelationGroup) []RelationType { return g.Types }
	relWith         = func(g RelationGroup, t []RelationType) RelationGroup { g.Types = t; return g }
	relationTypeKey = func(t RelationType) string { return t.ID }
)

// ParseKinds merges the default dictionary with a workspace one (nil when the
// workspace has none) by the rule of CONTRACT §6, for kinds and for relation
// types alike.
func ParseKinds(defaultJSON, workspaceJSON []byte) (*KindCatalog, error) {
	base, err := parseKindsDoc("kinds.json (default)", defaultJSON)
	if err != nil {
		return nil, err
	}
	extra, err := parseKindsDoc(KindsFile, workspaceJSON)
	if err != nil {
		return nil, err
	}
	for _, chk := range []struct {
		name string
		doc  *kindsDoc
	}{{"kinds.json (default)", base}, {KindsFile, extra}} {
		if err := uniqueIDs("kind", chk.name, chk.doc.Groups, kindGroupID, kindEntries, kindID); err != nil {
			return nil, err
		}
		if err := uniqueIDs("relation type", chk.name, chk.doc.RelationGroups, relGroupID, relEntries, relationTypeKey); err != nil {
			return nil, err
		}
	}
	c := &KindCatalog{
		Groups:         mergeGroups(base.Groups, extra.Groups, kindGroupID, kindEntries, kindWith, kindID),
		RelationGroups: mergeGroups(base.RelationGroups, extra.RelationGroups, relGroupID, relEntries, relWith, relationTypeKey),
	}
	// a replaced default group took its entries with it; the rest must still be unique
	if err := uniqueIDs("kind", "kinds", c.Groups, kindGroupID, kindEntries, kindID); err != nil {
		return nil, err
	}
	if err := uniqueIDs("relation type", "kinds", c.RelationGroups, relGroupID, relEntries, relationTypeKey); err != nil {
		return nil, err
	}
	c.reindex()
	return c, nil
}

func (c *KindCatalog) reindex() {
	c.index = map[string]Kind{}
	for _, g := range c.Groups {
		for _, k := range g.Kinds {
			c.index[k.ID] = k
		}
	}
	c.relIndex = map[string]RelationType{}
	for _, g := range c.RelationGroups {
		for _, t := range g.Types {
			c.relIndex[t.ID] = t
		}
	}
}

// LoadKinds reads <workspace>/kinds.json, when there is one, over the default.
func LoadKinds(workspace string, defaultJSON []byte) (*KindCatalog, error) {
	data, err := os.ReadFile(filepath.Join(workspace, KindsFile))
	if errors.Is(err, fs.ErrNotExist) {
		data = nil
	} else if err != nil {
		return nil, err
	}
	return ParseKinds(defaultJSON, data)
}

// Lookup is the kind of that id; false when the catalog does not have it. The
// id is matched as it is: `abstract-class` and `class` are two entries.
func (c *KindCatalog) Lookup(kind string) (Kind, bool) {
	if c == nil {
		return Kind{}, false
	}
	k, ok := c.index[kind]
	return k, ok
}

// IsContainer: an entity of this kind is a container (CONTRACT §6, §8.2).
func (c *KindCatalog) IsContainer(kind string) bool {
	k, ok := c.Lookup(kind)
	return ok && k.Container
}

// LookupRelation is the relation type of that id; false when the catalog does
// not have it.
func (c *KindCatalog) LookupRelation(relationType string) (RelationType, bool) {
	if c == nil {
		return RelationType{}, false
	}
	t, ok := c.relIndex[relationType]
	return t, ok
}

// Localized picks lang, else any language present (the first by code, so the
// answer does not change from call to call).
func Localized(m map[string]string, lang string) string {
	if v, ok := m[lang]; ok && v != "" {
		return v
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if m[k] != "" {
			return m[k]
		}
	}
	return ""
}
