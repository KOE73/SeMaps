package core

import (
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// arrange_like: "make the other zones like this one". The reference zone's
// layout is carried onto each target zone; nodes are paired by their part in the
// inheritance of the zone, then by the words that tell one variant from another.

type ArrangeOptions struct {
	Reference string   // reference to the model zone: `view#zone`
	Targets   []string // references or ids of the zones to lay out
	Parent    string   // instead of Targets: every zone next to the reference inside this zone; they are also restacked
	DryRun    bool
}

type NodePair struct {
	Reference string `json:"reference"`
	Target    string `json:"target"`
}

type ArrangeTarget struct {
	Zone      string     `json:"zone"`
	Pairs     []NodePair `json:"pairs"`
	Unpaired  []string   `json:"unpaired"`
	Missing   []string   `json:"missing"`
	Rect      Rect       `json:"rect"`
	Restacked bool       `json:"restacked,omitempty"`
}

type ArrangeReport struct {
	GeomReport
	DryRun  bool            `json:"dryRun"`
	Targets []ArrangeTarget `json:"targets"`
}

func (m *Model) ArrangeLike(view string, opt ArrangeOptions, human bool, author string) (ArrangeReport, error) {
	if !human {
		return ArrangeReport{}, refuse(needHuman)
	}
	l, err := m.loadLayout(view)
	if err != nil {
		return ArrangeReport{}, err
	}
	ref, err := l.id(opt.Reference)
	if err != nil {
		return ArrangeReport{}, err
	}
	if !l.isZone(ref) {
		return ArrangeReport{}, refuse("reference %s is not a zone", ref)
	}
	var targets []string
	parent := ""
	switch {
	case len(opt.Targets) > 0 && opt.Parent != "":
		return ArrangeReport{}, refuse("give targets or parent, not both")
	case opt.Parent != "":
		if parent, err = l.id(opt.Parent); err != nil {
			return ArrangeReport{}, err
		}
		if !l.isZone(parent) {
			return ArrangeReport{}, refuse("parent %s is not a zone", parent)
		}
		for _, id := range l.zoneOrder {
			if id != ref && l.parents[id] == parent {
				targets = append(targets, id)
			}
		}
		if l.parents[ref] != parent {
			return ArrangeReport{}, refuse("reference %s is not inside %s", ref, parent)
		}
	case len(opt.Targets) > 0:
		if targets, err = l.ids(opt.Targets); err != nil {
			return ArrangeReport{}, err
		}
	default:
		return ArrangeReport{}, refuse("give targets or parent")
	}
	if len(targets) == 0 {
		return ArrangeReport{}, refuse("no target zones")
	}
	for _, t := range targets {
		switch {
		case !l.isZone(t):
			return ArrangeReport{}, refuse("target %s is not a zone", t)
		case t == ref:
			return ArrangeReport{}, refuse("the reference zone cannot be its own target")
		case slices.Contains(l.subtree(ref), t) || slices.Contains(l.subtree(t), ref):
			return ArrangeReport{}, refuse("%s and %s are nested in each other", ref, t)
		}
	}

	extends := m.extendsIn()
	refZone := l.rect(ref)
	refNodes := l.zoneNodes(ref)
	report := ArrangeReport{DryRun: opt.DryRun}
	original := map[string]Rect{}
	for _, id := range append([]string{ref}, targets...) {
		original[id] = l.rect(id)
	}
	for _, t := range targets {
		tz := l.rect(t)
		tNodes := l.zoneNodes(t)
		pairs := pairNodes(refNodes, tNodes, extends)
		at := ArrangeTarget{Zone: t, Pairs: []NodePair{}, Unpaired: []string{}, Missing: []string{}}
		paired := map[string]bool{}
		refPaired := map[string]bool{}
		for _, p := range pairs {
			paired[p.Target], refPaired[p.Reference] = true, true
			rn, tn := l.nodes[p.Reference], l.nodes[p.Target]
			rr := l.rect(p.Reference)
			l.setRect(p.Target, Rect{Snap(tz.X + rr.X - refZone.X), Snap(tz.Y + rr.Y - refZone.Y), rr.Width, rr.Height})
			copyKey(rn, tn, "template")
			copyKey(rn, tn, "styleId")
			at.Pairs = append(at.Pairs, p)
		}
		for _, id := range refNodes {
			if !refPaired[id] {
				at.Missing = append(at.Missing, id)
			}
		}
		// what has no counterpart goes to a free row under the layout
		rowY := tz.Y + refZone.Height
		for _, id := range tNodes {
			if !paired[id] {
				rowY = math.Max(rowY, l.rect(id).Bottom()+ZonePadding)
			}
		}
		rowY = math.Ceil(rowY/GridStep) * GridStep
		x := tz.X + ZonePadding
		for _, id := range tNodes {
			if paired[id] {
				continue
			}
			r := l.rect(id)
			l.setRect(id, Rect{Snap(x), rowY, r.Width, r.Height})
			x += r.Width + 20
			at.Unpaired = append(at.Unpaired, id)
		}
		// the zone: the reference's size (more, if the free row needs it), style, fold
		zr := l.keepContent(t, Rect{tz.X, tz.Y, refZone.Width, refZone.Height})
		l.setRect(t, zr)
		copyKey(l.zones[ref], l.zones[t], "styleId")
		copyKey(l.zones[ref], l.zones[t], "collapsed")
		at.Rect = zr
		report.Targets = append(report.Targets, at)
	}

	if parent != "" {
		l.restack(ref, targets, original, &report)
	}
	for _, t := range targets {
		l.growAncestors(t)
	}
	if parent != "" {
		l.growAncestors(ref)
	}
	for i := range report.Targets {
		report.Targets[i].Rect = l.rect(report.Targets[i].Zone)
	}
	rep, err := m.commitMode(l, nil, author, human, opt.DryRun)
	report.GeomReport = rep
	return report, err
}

// copyKey puts the value of key in from onto to, when from has it.
func copyKey(from, to *object, key string) {
	v, ok := from.vals[key]
	if !ok {
		return
	}
	if _, has := to.vals[key]; !has {
		to.keys = append(to.keys, key)
	}
	to.vals[key] = v
}

// zoneNodes: the nodes that are directly in a zone, in file order.
func (l *layout) zoneNodes(zone string) []string {
	var out []string
	for _, id := range l.nodeOrder {
		if nodeZone(l.nodes[id]) == zone {
			out = append(out, id)
		}
	}
	return out
}

// extendsIn: for every entity, the entities it extends.
func (m *Model) extendsIn() map[string][]string {
	out := map[string][]string{}
	for _, r := range m.records("relation") {
		if relationType(r) == "extends" {
			out[r.str("from")] = append(out[r.str("from")], r.str("to"))
		}
	}
	return out
}

// pairNodes pairs the nodes of the reference zone with those of a target. First
// the bases of a family (the node the others of the zone extend), then the rest
// by the words that tell the variants apart (fp16, u8, nhwc…), best match first.
func pairNodes(ref, target []string, extends map[string][]string) []NodePair {
	var pairs []NodePair
	usedR, usedT := map[string]bool{}, map[string]bool{}
	take := func(r, t string) {
		pairs = append(pairs, NodePair{r, t})
		usedR[r], usedT[t] = true, true
	}
	bases := func(nodes []string) []string {
		var out []string
		for _, n := range nodes {
			for _, other := range nodes {
				if slices.Contains(extends[other], n) {
					out = append(out, n)
					break
				}
			}
		}
		return out
	}
	rb, tb := bases(ref), bases(target)
	for i := 0; i < len(rb) && i < len(tb); i++ {
		take(rb[i], tb[i])
	}
	rt, tt := distinguishing(ref), distinguishing(target)
	type cand struct {
		r, t   string
		score  float64
		ri, ti int
	}
	var cands []cand
	for ri, r := range ref {
		for ti, t := range target {
			if usedR[r] || usedT[t] {
				continue
			}
			if sc := similarity(rt[r], tt[t]); sc > 0 {
				cands = append(cands, cand{r, t, sc, ri, ti})
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.ri != b.ri {
			return a.ri < b.ri
		}
		return a.ti < b.ti
	})
	for _, c := range cands {
		if !usedR[c.r] && !usedT[c.t] {
			take(c.r, c.t)
		}
	}
	return pairs
}

// distinguishing: for each entity the words of its name that not every node of the zone has.
func distinguishing(nodes []string) map[string]map[string]bool {
	words := map[string]map[string]bool{}
	count := map[string]int{}
	for _, n := range nodes {
		ws := map[string]bool{}
		for _, w := range splitWords(n) {
			ws[w] = true
		}
		words[n] = ws
		for w := range ws {
			count[w]++
		}
	}
	if len(nodes) > 1 {
		for _, ws := range words {
			for w := range ws {
				if count[w] == len(nodes) {
					delete(ws, w)
				}
			}
		}
	}
	return words
}

func similarity(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	common := 0
	for w := range a {
		if b[w] {
			common++
		}
	}
	return float64(common) / math.Max(float64(len(a)), float64(len(b)))
}

// splitWords cuts an entity id like e_op_onnx_crop_fp16_nchw at _ and at
// lower-to-upper humps, lower-cased: "RgbFP16Nchw" is rgb, fp16, nchw.
func splitWords(id string) []string {
	id = strings.TrimPrefix(id, "e_")
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	for i, r := range id {
		switch {
		case r == '_' || r == '.' || r == '-':
			flush()
		case unicode.IsUpper(r) && i > 0 && len(cur) > 0 && unicode.IsLower(cur[len(cur)-1]):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return out
}

// restack lays the reference and the targets out one after another in the
// direction they already stand in, with the gap the reference has to its
// neighbour. The first of them stays where it is; the rest follow.
func (l *layout) restack(ref string, targets []string, original map[string]Rect, report *ArrangeReport) {
	all := append([]string{ref}, targets...)
	minX, maxX := math.Inf(1), math.Inf(-1)
	for _, id := range all {
		minX, maxX = math.Min(minX, original[id].X), math.Max(maxX, original[id].X)
	}
	vertical := maxX-minX < original[ref].Width
	pos := func(r Rect) float64 {
		if vertical {
			return r.Y
		}
		return r.X
	}
	sort.SliceStable(all, func(i, j int) bool { return pos(original[all[i]]) < pos(original[all[j]]) })
	gap := ZonePadding + 4
	for i, id := range all {
		if id != ref {
			continue
		}
		var g float64
		switch {
		case i+1 < len(all):
			n := original[all[i+1]]
			g = pos(n) - map[bool]float64{true: original[ref].Bottom(), false: original[ref].Right()}[vertical]
		case i > 0:
			p := original[all[i-1]]
			g = pos(original[ref]) - map[bool]float64{true: p.Bottom(), false: p.Right()}[vertical]
		default:
			g = -1
		}
		if g >= 0 {
			gap = g
		}
	}
	gap = Snap(gap)
	at := slices.Index(all, ref)
	moved := func(id string) {
		for i := range report.Targets {
			if report.Targets[i].Zone == id {
				report.Targets[i].Restacked = true
			}
		}
	}
	// the reference stays; what followed it in the row follows again, what preceded it goes back
	anchor := l.rect(ref)
	for _, id := range all[at+1:] {
		r := l.rect(id)
		if vertical {
			l.shift([]string{id}, anchor.X-r.X, anchor.Bottom()+gap-r.Y)
		} else {
			l.shift([]string{id}, anchor.Right()+gap-r.X, anchor.Y-r.Y)
		}
		anchor = l.rect(id)
		moved(id)
	}
	anchor = l.rect(ref)
	for i := at - 1; i >= 0; i-- {
		id := all[i]
		r := l.rect(id)
		if vertical {
			l.shift([]string{id}, anchor.X-r.X, anchor.Y-gap-r.Height-r.Y)
		} else {
			l.shift([]string{id}, anchor.X-gap-r.Width-r.X, anchor.Y-r.Y)
		}
		anchor = l.rect(id)
		moved(id)
	}
}
