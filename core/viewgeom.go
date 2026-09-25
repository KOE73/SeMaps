package core

import (
	"encoding/json"
	"math"
)

// The numbers of docs/LAYOUT.md, in one place. The editor uses the same ones.
const (
	GridStep      = 10.0
	NodeWidth     = 180.0
	NodeHeight    = 60.0
	MinNodeWidth  = 100.0
	MinNodeHeight = 40.0
	MinZoneWidth  = 160.0
	MinZoneHeight = 100.0
	ZoneHeader    = 28.0
	ZonePadding   = 16.0
)

// Rect is a box in the absolute model coordinates of a view.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (r Rect) Right() float64  { return r.X + r.Width }
func (r Rect) Bottom() float64 { return r.Y + r.Height }

func (r Rect) Union(o Rect) Rect {
	x, y := math.Min(r.X, o.X), math.Min(r.Y, o.Y)
	return Rect{x, y, math.Max(r.Right(), o.Right()) - x, math.Max(r.Bottom(), o.Bottom()) - y}
}

func (r Rect) Contains(o Rect) bool {
	return o.X >= r.X && o.Y >= r.Y && o.Right() <= r.Right() && o.Bottom() <= r.Bottom()
}

// Snap rounds to the grid.
func Snap(v float64) float64 { return math.Round(v/GridStep) * GridStep }

func (o *object) num(key string) (float64, bool) {
	var f float64
	if raw, ok := o.vals[key]; ok && json.Unmarshal(raw, &f) == nil {
		return f, true
	}
	return 0, false
}

func (o *object) numOr(key string, def float64) float64 {
	if f, ok := o.num(key); ok {
		return f
	}
	return def
}

// zoneRect and nodeRect read the box of an object; a node without a size has the default one.
func zoneRect(o *object) Rect {
	return Rect{o.numOr("x", 0), o.numOr("y", 0), o.numOr("width", MinZoneWidth), o.numOr("height", MinZoneHeight)}
}

func nodeRect(o *object) Rect {
	return Rect{o.numOr("x", 0), o.numOr("y", 0), o.numOr("width", NodeWidth), o.numOr("height", NodeHeight)}
}

func nodeZone(o *object) string { return orDefault(o.str("zone"), o.str("container")) }
