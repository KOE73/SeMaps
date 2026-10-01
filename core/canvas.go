package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CanvasFile is the workspace-overridable default that holds the numbers of the
// canvas (docs/API.md §2.1a). The tool ships one (host/defaults/canvas.json);
// a workspace copy wins. It is the ONE place these numbers are written down:
// the host reads it and hands a Canvas to the model, the editor reads the same
// file.
const CanvasFile = "canvas.json"

// Canvas: grid, sizes and gaps of a view, in model units (1 unit = 1 screen
// pixel at zoom 1). The editor snaps to Grid; the host does not.
type Canvas struct {
	Grid float64 `json:"grid"`
	Node struct {
		Width     float64 `json:"width"`
		Height    float64 `json:"height"`
		MinWidth  float64 `json:"minWidth"`
		MinHeight float64 `json:"minHeight"`
		Radius    float64 `json:"radius"`
	} `json:"node"`
	Container struct {
		MinWidth     float64 `json:"minWidth"`
		MinHeight    float64 `json:"minHeight"`
		HeaderHeight float64 `json:"headerHeight"`
		Padding      float64 `json:"padding"`
		Radius       float64 `json:"radius"`
	} `json:"container"`
	Gap struct {
		Node      float64 `json:"node"`
		Container float64 `json:"container"`
	} `json:"gap"`
}

// ParseCanvas reads canvas.json and checks that the numbers can be worked with.
func ParseCanvas(data []byte) (Canvas, error) {
	var c Canvas
	if err := json.Unmarshal(data, &c); err != nil {
		return Canvas{}, fmt.Errorf("%s: %w", CanvasFile, err)
	}
	for name, v := range map[string]float64{
		"grid": c.Grid, "node.width": c.Node.Width, "node.height": c.Node.Height,
		"node.minWidth": c.Node.MinWidth, "node.minHeight": c.Node.MinHeight,
		"container.minWidth": c.Container.MinWidth, "container.minHeight": c.Container.MinHeight,
	} {
		if v <= 0 {
			return Canvas{}, fmt.Errorf("%s: %s must be a positive number", CanvasFile, name)
		}
	}
	for name, v := range map[string]float64{
		"node.radius": c.Node.Radius, "container.headerHeight": c.Container.HeaderHeight, "container.padding": c.Container.Padding,
		"container.radius": c.Container.Radius, "gap.node": c.Gap.Node, "gap.container": c.Gap.Container,
	} {
		if v < 0 {
			return Canvas{}, fmt.Errorf("%s: %s must not be negative", CanvasFile, name)
		}
	}
	return c, nil
}

// LoadCanvas: <workspace>/canvas.json when it exists, otherwise the tool's
// default, given as bytes by the caller (the host embeds it).
func LoadCanvas(workspace string, fallback []byte) (Canvas, error) {
	data, err := os.ReadFile(filepath.Join(workspace, CanvasFile))
	if errors.Is(err, fs.ErrNotExist) {
		return ParseCanvas(fallback)
	}
	if err != nil {
		return Canvas{}, err
	}
	return ParseCanvas(data)
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Describe is the canvas in a few lines an agent reads before any geometry:
// the same text heads every get_view answer and is part of layout_guide.
func (c Canvas) Describe() string {
	g := num(c.Grid)
	var b strings.Builder
	b.WriteString("CANVAS. Units are model units (1 unit = 1 screen pixel at zoom 1); x grows right, y grows down, the origin is arbitrary; all coordinates are absolute.\n")
	fmt.Fprintf(&b, "Grid step %s: the editor snaps to it, this host does NOT — put positions and sizes on multiples of %s yourself.\n", g, g)
	fmt.Fprintf(&b, "Block: %sx%s by default, minimum %sx%s. Container: minimum %sx%s, caption strip %s high, inner padding %s.\n",
		num(c.Node.Width), num(c.Node.Height), num(c.Node.MinWidth), num(c.Node.MinHeight),
		num(c.Container.MinWidth), num(c.Container.MinHeight), num(c.Container.HeaderHeight), num(c.Container.Padding))
	fmt.Fprintf(&b, "Default gaps: %s between blocks, %s between containers.\n", num(c.Gap.Node), num(c.Gap.Container))
	return b.String()
}

// SetCanvas gives the model the canvas its geometry works with. The host does
// it every time it hands a model out, so a workspace override is picked up.
func (m *Model) SetCanvas(c Canvas) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.canvas = c
}

// Canvas is the canvas the model was given; an error when it has none.
func (m *Model) Canvas() (Canvas, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.canvas.Grid <= 0 {
		return Canvas{}, errors.New("the model has no canvas: the host gives it one (canvas.json)")
	}
	return m.canvas, nil
}
