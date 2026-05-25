package main

import (
	"context"
	"image/color"
	"log"
	"os"
	"os/exec"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type C = layout.Context
type D = layout.Dimensions

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

func center(gtx C, w layout.Widget) D {
    return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
        layout.Flexed(1, layout.Spacer{}.Layout),
        layout.Rigid(w),
        layout.Flexed(1, layout.Spacer{}.Layout),
    )
}

func run(window *app.Window) error {
	var cancelFn context.CancelFunc

	th := material.NewTheme()

	var loading atomic.Bool
	loader := material.Loader(th)

	ed := widget.Editor {SingleLine: true}
	editor := material.Editor(th, &ed, "URL")

	var downloadClickable widget.Clickable
	var cancelClickable widget.Clickable

	var ops op.Ops
	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			if downloadClickable.Clicked(gtx) && ed.Text() != "" {
				url := ed.Text()

				ctx, cancel := context.WithCancel(context.Background())
				cancelFn = cancel
				cmd := exec.CommandContext(ctx, "yt-dlp", url)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr

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
				margins := layout.Inset{
					// Top:    unit.Dp(25),
					// Bottom: unit.Dp(25),
					Right:  unit.Dp(35),
					Left:   unit.Dp(35),
				}

				padding := layout.Inset{
					Top:    unit.Dp(10),
					Bottom: unit.Dp(10),
					Right:  unit.Dp(10),
					Left:   unit.Dp(10),
				}

				border := widget.Border{
					Color:        color.NRGBA{R: 204, G: 204, B: 204, A: 255},
					CornerRadius: unit.Dp(3),
					Width:        unit.Dp(2),
				}

				return margins.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Dp(500)
					gtx.Constraints.Max.X = gtx.Dp(500)
					return border.Layout(gtx, func(gtx C) D {
						return padding.Layout(gtx, editor.Layout)
					})
				})
			}

			downloadBtn := material.Button(th, &downloadClickable, "Download")
			cancelBtn := material.Button(th, &cancelClickable, "Cancel")
			cancelBtn.Background = color.NRGBA{R: 220, G: 60, B: 60, A: 255}
			cancelBtn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

			form := func(gtx C) D {
				return layout.Flex{
					Axis: layout.Horizontal,
					Alignment: layout.Middle,
				}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D {
						return layout.Spacer{}.Layout(gtx)
					}),
					layout.Rigid(edLayout),
					layout.Rigid(func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(24))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(24))

						if loading.Load() {
							return loader.Layout(gtx)
						}

						return layout.Spacer{}.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
					    return layout.Inset{Left: unit.Dp(5)}.Layout(gtx, func(gtx C) D {
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
					layout.Flexed(1, func(gtx C) D {
						return layout.Spacer{}.Layout(gtx)
					}),
				)
			}

			layout.Flex{
				Axis: layout.Vertical,
				Spacing: layout.SpaceEnd,
			}.Layout(gtx,
				layout.Flexed(1, layout.Spacer{Height: unit.Dp(25)}.Layout),
				layout.Rigid(form),
				layout.Flexed(1, func(gtx C) D { return layout.Spacer{}.Layout(gtx) }),
			)


			e.Frame(gtx.Ops)
		}
	}
}
