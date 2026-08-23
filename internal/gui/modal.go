package gui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (ui *ui) layoutModal(gtx C) D {
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			ui.requestCloseModal()
		}
	}

	ui.mu.Lock()
	title := ui.modalTitle
	busy := ui.busy
	status := ui.status
	kind := ui.kind
	lines := append([]string(nil), ui.logLines...)
	ui.mu.Unlock()

	dim := color.NRGBA{R: 0, G: 0, B: 0, A: 0xb0}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			paint.Fill(gtx.Ops, dim)
			return ui.scrim.Layout(gtx, func(gtx C) D {
				return D{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx C) D {
			max := gtx.Constraints.Max
			width := max.X * 5 / 6
			if width > gtx.Dp(unit.Dp(720)) {
				width = gtx.Dp(unit.Dp(720))
			}
			height := max.Y * 3 / 4
			if height > gtx.Dp(unit.Dp(480)) {
				height = gtx.Dp(unit.Dp(480))
			}
			gtx.Constraints.Min = image.Pt(width, height)
			gtx.Constraints.Max = image.Pt(width, height)
			return rounded(gtx, 12, rgb(0x1a1d24), func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Inset{
							Top: unit.Dp(16), Bottom: unit.Dp(8),
							Left: unit.Dp(18), Right: unit.Dp(18),
						}.Layout(gtx, func(gtx C) D {
							l := material.H6(ui.th, title)
							l.Font.Weight = font.Medium
							return l.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx C) D {
							l := material.Caption(ui.th, status)
							l.Color = statusColor(kind)
							l.MaxLines = 2
							return l.Layout(gtx)
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx C) D {
						return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx C) D {
							return rounded(gtx, 8, rgb(0x111318), func(gtx C) D {
								return layout.Inset{
									Top: unit.Dp(8), Bottom: unit.Dp(8),
									Left: unit.Dp(10), Right: unit.Dp(10),
								}.Layout(gtx, func(gtx C) D {
									if len(lines) == 0 {
										l := material.Caption(ui.th, "Waiting for compiler output…")
										l.Color = rgb(0x7a8088)
										return l.Layout(gtx)
									}
									return material.List(ui.th, &ui.logList).Layout(gtx, len(lines), func(gtx C, i int) D {
										l := material.Caption(ui.th, lines[i])
										l.Color = rgb(0xc5cad3)
										l.Font.Typeface = "Go Mono"
										return l.Layout(gtx)
									})
								})
							})
						})
					}),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{
							Top: unit.Dp(12), Bottom: unit.Dp(14),
							Left: unit.Dp(18), Right: unit.Dp(18),
						}.Layout(gtx, func(gtx C) D {
							return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									label := "Close"
									if busy {
										label = "Cancel"
									}
									btn := material.Button(ui.th, &ui.close, label)
									btn.CornerRadius = unit.Dp(8)
									if busy {
										btn.Background = rgb(0x5c2b2b)
									}
									btn.Inset = layout.Inset{
										Top: unit.Dp(8), Bottom: unit.Dp(8),
										Left: unit.Dp(18), Right: unit.Dp(18),
									}
									return btn.Layout(gtx)
								}),
							)
						})
					}),
				)
			})
		}),
	)
}

func (ui *ui) requestCloseModal() {
	ui.mu.Lock()
	busy := ui.busy
	cancel := ui.cancel
	ui.mu.Unlock()
	if busy {
		if cancel != nil {
			cancel()
		}
		return
	}
	ui.mu.Lock()
	ui.showModal = false
	ui.mu.Unlock()
}
