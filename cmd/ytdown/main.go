package main

import (
	"bytes"
	"context"
	"image/color"
	"log"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type C = layout.Context
type D = layout.Dimensions

// doesn't actually write to editor because Gio widgets aren't thread safe
type EditorWriter struct {
    mu   sync.Mutex
    buf bytes.Buffer
    win  *app.Window
	dirty bool
}

func (w *EditorWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	b := &w.buf
	n, err = b.Write(p)
	w.mu.Unlock()

	if bytes.Contains(p, []byte("\n")) {
		w.dirty = true
		w.win.Invalidate()
	}

    return
}

func main() {
	go func() {
		window := new(app.Window)
		err := run(window)
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(window *app.Window) error {
	var cancelFn context.CancelFunc

	th := material.NewTheme()

	var loading atomic.Bool
	loader := material.Loader(th)

	ed := widget.Editor{SingleLine: true}
	editor := material.Editor(th, &ed, "URL")

	var outputEd widget.Editor
	outputEd.ReadOnly = true
	lw := &EditorWriter{win: window}

	var downloadClickable widget.Clickable
	var cancelClickable widget.Clickable
	downloadBtn := material.Button(th, &downloadClickable, "Download")
	cancelBtn := material.Button(th, &cancelClickable, "Cancel")
	cancelBtn.Background = color.NRGBA{R: 220, G: 60, B: 60, A: 255}
	cancelBtn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}


	var list widget.List
	list.Axis = layout.Vertical
	list.ScrollToEnd = true

	var ops op.Ops

	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			if downloadClickable.Clicked(gtx) && !loading.Load() && ed.Text() != "" {
				url := ed.Text()

				ctx, cancel := context.WithCancel(context.Background())
				cancelFn = cancel
				cmd := exec.CommandContext(ctx, "yt-dlp", url)
				cmd.Stdout = lw
				cmd.Stderr = lw

				err := cmd.Start()
				if err != nil {
					log.Fatal(err)
				}
				loading.Store(true)

				go func() {
					err := cmd.Wait()
					loading.Store(false)
					if err != nil {
						log.Println(err)
					}
					window.Invalidate()
				}()
			}

			if cancelClickable.Clicked(gtx) {
				if cancelFn != nil {
					cancelFn()
					cancelFn = nil
				}
			}

			edLayout := func(gtx C) D {
			    padding := layout.Inset{
			        Top: unit.Dp(10), Bottom: unit.Dp(10),
			        Right: unit.Dp(10), Left: unit.Dp(10),
			    }
			    border := widget.Border{
			        Color:        color.NRGBA{R: 204, G: 204, B: 204, A: 255},
			        CornerRadius: unit.Dp(3),
			        Width:        unit.Dp(2),
			    }
			    return border.Layout(gtx, func(gtx C) D {
			        return padding.Layout(gtx, editor.Layout)
			    })
			}

			form := func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			        layout.Flexed(1, edLayout),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			        layout.Rigid(func(gtx C) D {
			            gtx.Constraints.Min.X = gtx.Dp(24)
			            gtx.Constraints.Max.X = gtx.Dp(24)
			            if loading.Load() {
			                return loader.Layout(gtx)
			            }
			            return layout.Spacer{}.Layout(gtx)
			        }),
			        layout.Rigid(func(gtx C) D {
			            return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
			                if loading.Load() {
			                    gtx = gtx.Disabled()
			                }
			                return downloadBtn.Layout(gtx)
			            })
			        }),
			        layout.Rigid(func(gtx C) D {
			            return layout.Inset{Left: unit.Dp(5)}.Layout(gtx, func(gtx C) D {
			                if !loading.Load() {
			                    gtx = gtx.Disabled()
			                }
			                return cancelBtn.Layout(gtx)
			            })
			        }),
			    )
			}

		outputEditor := func(gtx C) D {
			termBg := color.NRGBA{R: 20, G: 20, B: 20, A: 255}
			termFg := color.NRGBA{R: 220, G: 220, B: 220, A: 255}

			// change background
			stack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			paint.Fill(gtx.Ops, termBg)
			stack.Pop()

			e := material.Editor(th, &outputEd, "")
			e.Color = termFg
			lst := material.List(th, &list)
			lst.Indicator.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
			lst.Track.Color = color.NRGBA{R: 60, G: 60, B: 60, A: 255}

			padding := layout.UniformInset(unit.Dp(15))

			lw.mu.Lock()
			if lw.dirty {
			    outputEd.SetText(lw.buf.String())
			    lw.dirty = false
			}
			lw.mu.Unlock()

			return lst.Layout(gtx, 1, func(gtx C, _ int) D {
				return padding.Layout(gtx, e.Layout)
			})
		}

		column := func(gtx C) D {
			width := min(gtx.Constraints.Max.X, gtx.Dp(800))

				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					gtx.Constraints.Min.X = width
					gtx.Constraints.Max.X = width
					return form(gtx)
				}),

				layout.Rigid(func(gtx C) D {
					gtx.Constraints.Min.X = width
					gtx.Constraints.Max.X = width
					gtx.Constraints.Max.Y = gtx.Dp(300)
					gtx.Constraints.Min.Y = gtx.Dp(300)

					return layout.Inset{Top: unit.Dp(15)}.Layout(gtx, outputEditor)
				}),
			)
		}

			layout.Center.Layout(gtx, column)

			e.Frame(gtx.Ops)
		}
	}
}
