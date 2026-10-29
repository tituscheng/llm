// Package gui implements a Gio window for compiling inference tools
// from upstream source into ~/.llm/tools.
package gui

import (
	"fmt"
	"image/color"
	"os"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"llm/install"
)

// Main opens the installer window. It must run on the program's main
// goroutine so the operating system can own the UI thread.
func Main() error {
	go func() {
		if err := run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
	return nil
}

func run() error {
	w := new(app.Window)
	w.Option(
		app.Title("llm — install binaries"),
		app.Size(unit.Dp(960), unit.Dp(640)),
		app.MinSize(unit.Dp(760), unit.Dp(500)),
	)

	dest, err := install.ToolsDir()
	if err != nil {
		return err
	}

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{
		Fg:         rgb(0xe8eaed),
		Bg:         rgb(0x111318),
		ContrastBg: rgb(0x4c8dff),
		ContrastFg: rgb(0xffffff),
	}

	ui := newUI(w, th, dest)
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			ui.stop()
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			ui.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{
		R: uint8(c >> 16),
		G: uint8(c >> 8),
		B: uint8(c),
		A: 0xff,
	}
}
