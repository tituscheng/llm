package gui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"path/filepath"
	"sync"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"llm/install"
)

type toolInstaller func(ctx context.Context, name string, stdout, stderr io.Writer) error

var runInstall toolInstaller = install.Install

type (
	C = layout.Context
	D = layout.Dimensions
)

type statusKind int

const (
	statusIdle statusKind = iota
	statusBusy
	statusOK
	statusErr
)

type ui struct {
	win  *app.Window
	th   *material.Theme
	dest string
	cats []install.Category

	category widget.Enum
	binary   widget.Enum
	install  widget.Clickable
	close    widget.Clickable
	scrim    widget.Clickable
	list     widget.List
	logList  widget.List

	mu         sync.Mutex
	busy       bool
	showModal  bool
	status     string
	kind       statusKind
	modalTitle string
	logLines   []string
	cancel     context.CancelFunc
	installWG  sync.WaitGroup
}

func newUI(win *app.Window, th *material.Theme, dest string) *ui {
	cats := install.Categories()
	u := &ui{
		win:    win,
		th:     th,
		dest:   dest,
		cats:   cats,
		status: "Select a category and binary, then Install.",
		list: widget.List{
			List: layout.List{Axis: layout.Vertical},
		},
		logList: widget.List{
			List: layout.List{Axis: layout.Vertical},
		},
	}
	if len(cats) > 0 {
		u.category.Value = cats[0].ID
		if len(cats[0].Tools) > 0 {
			u.binary.Value = cats[0].Tools[0].Name
		}
	}
	return u
}

func (ui *ui) Layout(gtx C) D {
	ui.mu.Lock()
	modal := ui.showModal
	ui.mu.Unlock()

	if !modal {
		if ui.category.Update(gtx) {
			ui.list.Position = layout.Position{}
			ui.syncBinary()
		}
		ui.binary.Update(gtx)
		ui.syncBinary()
		for ui.install.Clicked(gtx) {
			ui.startInstall()
		}
	} else {
		for ui.close.Clicked(gtx) {
			ui.requestCloseModal()
		}
		for ui.scrim.Clicked(gtx) {
			ui.requestCloseModal()
		}
	}

	paint.Fill(gtx.Ops, ui.th.Palette.Bg)
	main := func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(ui.layoutHeader),
			layout.Rigid(func(gtx C) D { return hairline(gtx, rgb(0x2a2e37)) }),
			layout.Flexed(1, ui.layoutBody),
			layout.Rigid(func(gtx C) D { return hairline(gtx, rgb(0x2a2e37)) }),
			layout.Rigid(ui.layoutFooter),
		)
	}
	if !modal {
		return main(gtx)
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(main),
		layout.Expanded(ui.layoutModal),
	)
}

func (ui *ui) layoutHeader(gtx C) D {
	return layout.Inset{
		Top: unit.Dp(20), Bottom: unit.Dp(16),
		Left: unit.Dp(24), Right: unit.Dp(24),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				l := material.H5(ui.th, "Install binaries")
				l.Font.Weight = font.Medium
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx C) D {
				l := material.Body1(ui.th, "Pick a category and binary. Install compiles the latest source into ~/.llm/tools.")
				l.Color = rgb(0x9aa0a6)
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx C) D {
				l := material.Caption(ui.th, "Destination  "+ui.dest)
				l.Color = rgb(0x7a8088)
				l.MaxLines = 1
				return l.Layout(gtx)
			}),
		)
	})
}

func (ui *ui) layoutBody(gtx C) D {
	return layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Dp(220)
			gtx.Constraints.Max.X = gtx.Dp(240)
			return ui.layoutCategories(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			return verticalRule(gtx, rgb(0x2a2e37))
		}),
		layout.Flexed(1, ui.layoutBinaries),
	)
}

func (ui *ui) layoutCategories(gtx C) D {
	return layout.Inset{
		Top: unit.Dp(16), Bottom: unit.Dp(16),
		Left: unit.Dp(16), Right: unit.Dp(12),
	}.Layout(gtx, func(gtx C) D {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx C) D {
				l := material.Caption(ui.th, "CATEGORY")
				l.Font.Weight = font.Medium
				l.Color = rgb(0x7a8088)
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
		}
		for i := range ui.cats {
			cat := ui.cats[i]
			children = append(children,
				layout.Rigid(func(gtx C) D {
					return ui.categoryButton(gtx, cat)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (ui *ui) categoryButton(gtx C, cat install.Category) D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	selected := ui.category.Value == cat.ID
	hovered, _ := ui.category.Hovered()
	bg := rgb(0x181b21)
	switch {
	case selected:
		bg = rgb(0x243044)
	case hovered == cat.ID:
		bg = rgb(0x1e222b)
	}

	count := fmt.Sprintf("%d", len(cat.Tools))
	if len(cat.Tools) == 1 {
		count += " binary"
	} else {
		count += " binaries"
	}

	return ui.category.Layout(gtx, cat.ID, func(gtx C) D {
		return rounded(gtx, 10, bg, func(gtx C) D {
			return layout.Inset{
				Top: unit.Dp(12), Bottom: unit.Dp(12),
				Left: unit.Dp(14), Right: unit.Dp(14),
			}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						l := material.Body1(ui.th, cat.Name)
						l.Font.Weight = font.Medium
						if selected {
							l.Color = ui.th.Palette.ContrastBg
						}
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
					layout.Rigid(func(gtx C) D {
						l := material.Caption(ui.th, cat.Description)
						l.Color = rgb(0x9aa0a6)
						l.MaxLines = 2
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx C) D {
						l := material.Caption(ui.th, count)
						l.Color = rgb(0x7a8088)
						return l.Layout(gtx)
					}),
				)
			})
		})
	})
}

func (ui *ui) layoutBinaries(gtx C) D {
	cat, ok := ui.currentCategory()
	if !ok {
		return D{}
	}
	bins := cat.Tools

	return layout.Inset{
		Top: unit.Dp(16), Bottom: unit.Dp(16),
		Left: unit.Dp(16), Right: unit.Dp(20),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				l := material.Caption(ui.th, "BINARY")
				l.Font.Weight = font.Medium
				l.Color = rgb(0x7a8088)
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Flexed(1, func(gtx C) D {
				if len(bins) == 0 {
					l := material.Body1(ui.th, "No binaries in this category.")
					l.Color = rgb(0x9aa0a6)
					return l.Layout(gtx)
				}
				return material.List(ui.th, &ui.list).Layout(gtx, len(bins), func(gtx C, i int) D {
					return layout.Inset{Bottom: unit.Dp(8)}.Layout(gtx, func(gtx C) D {
						return ui.binaryCard(gtx, bins[i])
					})
				})
			}),
		)
	})
}

func (ui *ui) binaryCard(gtx C, bin install.Tool) D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	selected := ui.binary.Value == bin.Name
	hovered, _ := ui.binary.Hovered()
	bg := rgb(0x181b21)
	switch {
	case selected:
		bg = rgb(0x243044)
	case hovered == bin.Name:
		bg = rgb(0x1e222b)
	}

	meta := "latest source"
	return ui.binary.Layout(gtx, bin.Name, func(gtx C) D {
		return rounded(gtx, 10, bg, func(gtx C) D {
			return layout.Inset{
				Top: unit.Dp(14), Bottom: unit.Dp(14),
				Left: unit.Dp(16), Right: unit.Dp(16),
			}.Layout(gtx, func(gtx C) D {
				children := []layout.FlexChild{
					layout.Rigid(func(gtx C) D {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								l := material.H6(ui.th, bin.Name)
								l.Font.Weight = font.Medium
								if selected {
									l.Color = ui.th.Palette.ContrastBg
								}
								return l.Layout(gtx)
							}),
							layout.Flexed(1, layout.Spacer{}.Layout),
							layout.Rigid(func(gtx C) D {
								if !selected {
									return D{}
								}
								l := material.Caption(ui.th, "SELECTED")
								l.Color = ui.th.Palette.ContrastBg
								l.Font.Weight = font.Medium
								return l.Layout(gtx)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx C) D {
						l := material.Caption(ui.th, meta)
						l.Color = rgb(0x9aa0a6)
						l.MaxLines = 2
						return l.Layout(gtx)
					}),
				}
				if bin.Note != "" {
					children = append(children,
						layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx C) D {
							l := material.Caption(ui.th, bin.Note)
							l.Color = rgb(0x7ab0ff)
							l.MaxLines = 2
							return l.Layout(gtx)
						}),
					)
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func (ui *ui) layoutFooter(gtx C) D {
	bin, ok := ui.selectedBinary()
	ui.mu.Lock()
	busy := ui.busy
	status := ui.status
	kind := ui.kind
	ui.mu.Unlock()

	summary := "No binary selected"
	destLine := ui.dest
	if ok {
		summary = bin.Name
		destLine = filepath.Join(ui.dest, bin.Name)
	}

	return layout.Inset{
		Top: unit.Dp(14), Bottom: unit.Dp(16),
		Left: unit.Dp(24), Right: unit.Dp(24),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								l := material.Body1(ui.th, summary)
								l.Font.Weight = font.Medium
								l.MaxLines = 1
								return l.Layout(gtx)
							}),
							layout.Rigid(func(gtx C) D {
								l := material.Caption(ui.th, destLine)
								l.Color = rgb(0x7a8088)
								l.MaxLines = 1
								return l.Layout(gtx)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx C) D {
						label := "Install"
						btnCtx := gtx
						if busy {
							label = "Installing…"
							btnCtx = btnCtx.Disabled()
						}
						if !ok {
							btnCtx = btnCtx.Disabled()
						}
						btn := material.Button(ui.th, &ui.install, label)
						btn.CornerRadius = unit.Dp(8)
						btn.Inset = layout.Inset{
							Top: unit.Dp(10), Bottom: unit.Dp(10),
							Left: unit.Dp(22), Right: unit.Dp(22),
						}
						return btn.Layout(btnCtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx C) D {
				l := material.Caption(ui.th, status)
				l.Color = statusColor(kind)
				l.MaxLines = 2
				return l.Layout(gtx)
			}),
		)
	})
}

func (ui *ui) currentCategory() (install.Category, bool) {
	for _, cat := range ui.cats {
		if cat.ID == ui.category.Value {
			return cat, true
		}
	}
	return install.Category{}, false
}

func (ui *ui) selectedBinary() (install.Tool, bool) {
	cat, ok := ui.currentCategory()
	if !ok {
		return install.Tool{}, false
	}
	return cat.Find(ui.binary.Value)
}

func (ui *ui) syncBinary() {
	cat, ok := ui.currentCategory()
	if !ok {
		ui.binary.Value = ""
		return
	}
	if _, ok := cat.Find(ui.binary.Value); ok {
		return
	}
	if len(cat.Tools) == 0 {
		ui.binary.Value = ""
		return
	}
	ui.binary.Value = cat.Tools[0].Name
}

func (ui *ui) appendLog(line string) {
	ui.mu.Lock()
	ui.logLines = appendCapped(ui.logLines, line)
	n := len(ui.logLines)
	ui.mu.Unlock()
	ui.logList.Position.First = n - 1
	if ui.logList.Position.First < 0 {
		ui.logList.Position.First = 0
	}
	ui.invalidate()
}

func (ui *ui) startInstall() {
	bin, ok := ui.selectedBinary()
	if !ok {
		ui.mu.Lock()
		ui.status = "Select a binary first."
		ui.kind = statusErr
		ui.mu.Unlock()
		return
	}

	ui.mu.Lock()
	if ui.busy {
		ui.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ui.busy = true
	ui.showModal = true
	ui.modalTitle = "Install — " + bin.Name
	ui.status = "Compiling latest " + bin.Name + "…"
	ui.kind = statusBusy
	ui.logLines = nil
	ui.logList.Position = layout.Position{}
	ui.cancel = cancel
	dest := ui.dest
	ui.installWG.Add(1)
	ui.mu.Unlock()

	out := &lineWriter{append: ui.appendLog}
	go func() {
		defer ui.installWG.Done()
		err := runInstall(ctx, bin.Name, out, out)
		out.flush()
		ui.mu.Lock()
		ui.busy = false
		ui.cancel = nil
		if ctx.Err() != nil {
			ui.status = "Cancelled."
			ui.kind = statusErr
		} else if err != nil {
			ui.status = err.Error()
			ui.kind = statusErr
		} else {
			ui.status = "Installed latest " + bin.Name + " to " + filepath.Join(dest, bin.Name)
			ui.kind = statusOK
		}
		ui.mu.Unlock()
		ui.invalidate()
	}()
}

func (ui *ui) invalidate() {
	if ui.win != nil {
		ui.win.Invalidate()
	}
}

// stop cancels an in-flight compile and waits until its process group is gone.
func (ui *ui) stop() {
	ui.mu.Lock()
	cancel := ui.cancel
	ui.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	ui.installWG.Wait()
}

func statusColor(kind statusKind) color.NRGBA {
	switch kind {
	case statusBusy:
		return rgb(0x7ab0ff)
	case statusOK:
		return rgb(0x81c995)
	case statusErr:
		return rgb(0xf28b82)
	default:
		return rgb(0x9aa0a6)
	}
}

func rounded(gtx C, r int, bg color.NRGBA, w layout.Widget) D {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()

	radius := gtx.Dp(unit.Dp(r))
	defer clip.UniformRRect(image.Rectangle{Max: dims.Size}, radius).Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, bg)
	call.Add(gtx.Ops)
	return dims
}

func hairline(gtx C, col color.NRGBA) D {
	h := gtx.Dp(1)
	rect := image.Rect(0, 0, gtx.Constraints.Max.X, h)
	paint.FillShape(gtx.Ops, col, clip.Rect(rect).Op())
	return D{Size: image.Pt(gtx.Constraints.Max.X, h)}
}

func verticalRule(gtx C, col color.NRGBA) D {
	w := gtx.Dp(1)
	rect := image.Rect(0, 0, w, gtx.Constraints.Max.Y)
	paint.FillShape(gtx.Ops, col, clip.Rect(rect).Op())
	return D{Size: image.Pt(w, gtx.Constraints.Max.Y)}
}
