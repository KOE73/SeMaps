package main

import (
	"log"

	"semaps/core"
)

// loadCanvas is the canvas of a workspace: <workspace>/canvas.json when it is
// there, otherwise the copy the tool ships (host/defaults/canvas.json, the one
// place the numbers are defined). A broken workspace copy is reported and
// replaced by the shipped one, so geometry keeps working.
func loadCanvas(workspace string) core.Canvas {
	def, err := bundled.ReadFile("defaults/" + core.CanvasFile)
	if err != nil {
		log.Fatalf("the tool's own %s is missing: %v", core.CanvasFile, err)
	}
	c, err := core.LoadCanvas(workspace, def)
	if err != nil {
		log.Printf("%v; using the tool's default", err)
		if c, err = core.ParseCanvas(def); err != nil {
			log.Fatalf("the tool's own %s is broken: %v", core.CanvasFile, err)
		}
	}
	return c
}
