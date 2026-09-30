package core

import (
	"encoding/json"
	"math"
)

// The numbers of the canvas (grid, block and container sizes, caption strip,
// padding) are not written here: they come from canvas.json as a Canvas
// (canvas.go), the one place they are defined; the editor reads the same file.
// The host does not snap to the grid — that is the editor's habit (the MCP tool
// `layout_guide` says so).

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

// placementRect reads the box of a placement; one without a size has the
// default size of its sort — a container's minimum or a block's default.
func placementRect(o *object, container bool, cv Canvas) Rect {
	if container {
		return Rect{o.numOr("x", 0), o.numOr("y", 0), o.numOr("width", cv.Container.MinWidth), o.numOr("height", cv.Container.MinHeight)}
	}
	return Rect{o.numOr("x", 0), o.numOr("y", 0), o.numOr("width", cv.Node.Width), o.numOr("height", cv.Node.Height)}
}
