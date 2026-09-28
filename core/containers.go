// Container membership: docs/CONTRACT.md §6. containers.json is not part of
// modelRegistries (core/model.go): the model never loads it, sync never
// touches it, and this file changes nothing about that — it only reads the
// file from a project directory and resolves it as a pure function.
package core

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ContainerMatch is one container's rule (CONTRACT §6). Every field is
// optional; an empty ContainerMatch matches nothing on its own.
type ContainerMatch struct {
	Name      []string `json:"name,omitempty"`
	NameRegex []string `json:"nameRegex,omitempty"`
	Path      []string `json:"path,omitempty"`
}

// Container is one entry of containers.json.
type Container struct {
	ID     string         `json:"id"`
	Parent string         `json:"parent,omitempty"`
	Theme  string         `json:"theme,omitempty"`
	Match  ContainerMatch `json:"match,omitempty"`
}

// Containers is the parsed shape of containers.json.
type Containers struct {
	ContractVersion int               `json:"contractVersion,omitempty"`
	List            []Container       `json:"containers"`
	Overrides       map[string]string `json:"overrides,omitempty"`
}

// LoadContainers reads containers.json from a project directory (Model.ProjectDir()).
// nil, nil when the file does not exist: a project without containers.json has
// no containment, not an error.
func LoadContainers(projectDir string) (*Containers, error) {
	o, err := loadDoc(filepath.Join(projectDir, "containers.json"))
	if err != nil || o == nil {
		return nil, err
	}
	b, err := o.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var c Containers
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ContainerSubject is the minimal input the resolver needs: enough to serve
// a registry entity (id, name, codeRef) and a code-only symbol (its own id,
// name, file) alike.
type ContainerSubject struct {
	ID   string // entity id, or a synthesized key for a code-only symbol
	Name string
	File string // codeRef/file without a `#`/`:` anchor
}

// ResolveContainers assigns every subject its containers, in the order of
// CONTRACT §6: `overrides` → `match.name` → `match.nameRegex` →
// `match.path` (longest prefix) → neighbour in the same file.
//
// A subject may end up in several containers: several containers can match
// the same name, or tie for the longest path prefix. Read literally, "an
// entity may belong to several containers" is about the rules yielding more
// than one match at the tier that decides it — resolution does not fall
// through to a weaker tier once a tier has any match.
//
// `overrides` is read as replacing rule resolution for that subject, not
// adding to it: CONTRACT §6 calls it "an explicit assignment by a human"
// that "always beats a rule", i.e. the human's word is the whole answer for
// that subject, not one more candidate among the rules'.
//
// "Neighbour in the same file" is read literally as: a subject with no rule
// match takes the containers of another subject in the same batch that
// *did* resolve (by any tier, override included) and shares its file. When
// several such neighbours disagree, all of their containers apply — the
// contract does not say to prefer one file-mate over another.
//
// The result lists container ids in containers.json order, deterministic.
func ResolveContainers(defs []Container, overrides map[string]string, subjects []ContainerSubject) map[string][]string {
	order := make(map[string]int, len(defs))
	for i, c := range defs {
		order[c.ID] = i
	}
	byName := map[string][]int{}
	byRegex := []struct {
		re  *regexp.Regexp
		idx int
	}{}
	for i, c := range defs {
		for _, n := range c.Match.Name {
			byName[n] = append(byName[n], i)
		}
		for _, pat := range c.Match.NameRegex {
			if re, err := regexp.Compile(pat); err == nil {
				byRegex = append(byRegex, struct {
					re  *regexp.Regexp
					idx int
				}{re, i})
			}
		}
	}

	sortIDs := func(ids []string) []string {
		sort.Slice(ids, func(a, b int) bool { return order[ids[a]] < order[ids[b]] })
		return ids
	}
	uniqueIDs := func(idxs []int) []string {
		seen := map[string]bool{}
		var out []string
		for _, i := range idxs {
			id := defs[i].ID
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		return sortIDs(out)
	}

	// ------------------------------------------------------ tier resolution
	resolveByRules := func(s ContainerSubject) []string {
		if id, ok := overrides[s.ID]; ok && id != "" {
			return []string{id}
		}
		if idxs := byName[s.Name]; len(idxs) > 0 {
			return uniqueIDs(idxs)
		}
		var reMatches []int
		for _, r := range byRegex {
			if r.re.MatchString(s.Name) {
				reMatches = append(reMatches, r.idx)
			}
		}
		if len(reMatches) > 0 {
			return uniqueIDs(reMatches)
		}
		if s.File != "" {
			longest := -1
			var pathMatches []int
			for i, c := range defs {
				for _, p := range c.Match.Path {
					if p == "" || !strings.HasPrefix(s.File, p) {
						continue
					}
					switch {
					case len(p) > longest:
						longest = len(p)
						pathMatches = []int{i}
					case len(p) == longest:
						pathMatches = append(pathMatches, i)
					}
				}
			}
			if len(pathMatches) > 0 {
				return uniqueIDs(pathMatches)
			}
		}
		return nil
	}

	result := make(map[string][]string, len(subjects))
	fileContainers := map[string]map[string]bool{} // file -> set of container ids, from resolved subjects
	var unresolved []ContainerSubject
	for _, s := range subjects {
		if ids := resolveByRules(s); len(ids) > 0 {
			result[s.ID] = ids
			if s.File != "" {
				set := fileContainers[s.File]
				if set == nil {
					set = map[string]bool{}
					fileContainers[s.File] = set
				}
				for _, id := range ids {
					set[id] = true
				}
			}
		} else {
			unresolved = append(unresolved, s)
		}
	}

	// ------------------------------------------------ tier 4: file neighbour
	for _, s := range unresolved {
		if s.File == "" {
			continue
		}
		set := fileContainers[s.File]
		if len(set) == 0 {
			continue
		}
		ids := make([]string, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		result[s.ID] = sortIDs(ids)
	}

	return result
}
