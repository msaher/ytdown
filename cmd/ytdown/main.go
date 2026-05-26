package main

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	"github.com/sqweek/dialog"
)

type C = layout.Context
type D = layout.Dimensions

// doesn't actually write to editor because Gio widgets aren't thread safe.
// Insetad we use a dirty flag and call win.Invalidate()
type EditorWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	win   *app.Window
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

func run(window *app.Window) error {

	lw := &EditorWriter{win: window}
	var loading atomic.Bool
	var ytDlpPath string // thread-safe because guraded by `loading`

	// ensure yt-dlp exists
	go func() {
		loading.Store(true)
		io.WriteString(lw, "checking yt-dlp installation...\n")
		pth, err := ensureYtDlp(lw)
		if err != nil {
			log.Println(err)
			io.WriteString(lw, err.Error()+"\n")
		} else {
			io.WriteString(lw, "yt-dlp Ready!\n")
			ytDlpPath = pth
		}
		loading.Store(false)
		window.Invalidate()
	}()

	var cancelFn context.CancelFunc
	th := material.NewTheme()

	ed := widget.Editor{SingleLine: true}
	outputEd := widget.Editor{ReadOnly: true}

	var downloadBtn widget.Clickable
	var cancelBtn widget.Clickable

	var list widget.List
	list.Axis = layout.Vertical
	list.ScrollToEnd = false

	var audioOnly widget.Bool

	var browseBtn widget.Clickable

	outputDir, _ := desktopPath()

	var ops op.Ops

	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			if downloadBtn.Clicked(gtx) && !loading.Load() && ed.Text() != "" && ytDlpPath != "" {
				url := ed.Text()

				ctx, cancel := context.WithCancel(context.Background())
				cancelFn = cancel

				// build cmd
				args := []string{
					ytDlpPath,
					"--embed-thumbnail",
					"--embed-metadata",
					"--embed-chapters",
					"--embed-info-json",
					"--xattrs",
				}
				if audioOnly.Value {
					args = append(args, "-x")
				}
				args = append(args, url)
				cmd := exec.CommandContext(ctx, args[0], args[1:]...)
				cmd.Stdout = lw
				cmd.Stderr = lw
				cmd.Dir = outputDir

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

			if cancelBtn.Clicked(gtx) {
				if cancelFn != nil {
					cancelFn()
					cancelFn = nil
				}
			}

			if browseBtn.Clicked(gtx) {
				go func() {
					directory, err := dialog.Directory().SetStartDir(outputDir).Browse()
					if nil == err {
						outputDir = directory
						window.Invalidate()
					}
				}()
			}

			edLayout := func(gtx C) D {
				e := material.Editor(th, &ed, "URL")
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
					return padding.Layout(gtx, e.Layout)
				})
			}

			row1 := func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, edLayout),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Dp(24)
						gtx.Constraints.Max.X = gtx.Dp(24)
						if loading.Load() {
							return material.Loader(th).Layout(gtx)
						}
						return layout.Spacer{}.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
							if loading.Load() {
								gtx = gtx.Disabled()
							}
							btn := material.Button(th, &downloadBtn, "Download")
							return btn.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{Left: unit.Dp(5)}.Layout(gtx, func(gtx C) D {
							if !loading.Load() {
								gtx = gtx.Disabled()
							}

							btn := material.Button(th, &cancelBtn, "Cancel")
							btn.Background = color.NRGBA{R: 220, G: 60, B: 60, A: 255}
							btn.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

							return btn.Layout(gtx)
						})
					}),
				)
			}

			row2 := func(gtx C) D {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Spacing:   layout.SpaceBetween,
					Alignment: layout.Middle,
					Gap:       gtx.Dp(10),
				}.Layout(gtx,
					layout.Flexed(0.4, func(gtx C) D {

						padding := layout.Inset{
							Top: unit.Dp(10), Bottom: unit.Dp(10),
							Right: unit.Dp(10), Left: unit.Dp(10),
						}
						border := widget.Border{
							Color:        color.NRGBA{R: 204, G: 204, B: 204, A: 255},
							CornerRadius: unit.Dp(3),
							Width:        unit.Dp(2),
						}

						e := material.H6(th, outputDir)
						return border.Layout(gtx, func(gtx C) D {
							return padding.Layout(gtx, e.Layout)
						})
					}),
					layout.Rigid(func(gtx C) D {
						btn := material.Button(th, &browseBtn, "Browse")
						return btn.Layout(gtx)
					}),
					layout.Flexed(0.3, material.CheckBox(th, &audioOnly, "Audio only").Layout),
				)
			}

			form := func(gtx C) D {
				return layout.Flex{
					Axis: layout.Vertical,
					Gap:  gtx.Dp(10),
				}.Layout(gtx,
					layout.Rigid(row1),
					layout.Rigid(row2),
				)
			}

			output := func(gtx C) D {
				// change background
				termBg := color.NRGBA{R: 20, G: 20, B: 20, A: 255}
				termFg := color.NRGBA{R: 220, G: 220, B: 220, A: 255}
				e := material.Editor(th, &outputEd, "")
				e.Color = termFg

				stack := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
				paint.Fill(gtx.Ops, termBg)
				stack.Pop()

				padding := layout.UniformInset(unit.Dp(15))

				lw.mu.Lock()
				if lw.dirty {
					outputEd.SetText(lw.buf.String())
					lw.dirty = false
				}
				lw.mu.Unlock()

				lst := material.List(th, &list)
				lst.Indicator.Color = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
				lst.Track.Color = color.NRGBA{R: 60, G: 60, B: 60, A: 255}
				return lst.Layout(gtx, 1, func(gtx C, _ int) D {
					return padding.Layout(gtx, e.Layout)
				})
			}

			column := func(gtx C) D {
				width := min(gtx.Constraints.Max.X, gtx.Dp(800))
				return layout.Flex{
					Axis:      layout.Vertical,
					Alignment: layout.Middle,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						gtx.Constraints.Min.X = 0
						h := material.H3(th, "YTDown")
						margin := layout.Inset{Bottom: unit.Dp(50)}
						return margin.Layout(gtx, h.Layout)
					}),
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
						return layout.Inset{Top: unit.Dp(15)}.Layout(gtx, output)
					}),
				)
			}
			layout.Center.Layout(gtx, column)
			e.Frame(gtx.Ops)
		}
	}
}

func desktopPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// try to put it in Dkestop. If not fallback to home directory
	desk := filepath.Join(home, "Desktop")
	_, err = os.Stat(desk)
	if err != nil {
		return home, nil
	}
	return desk, nil
}

func ensureYtDlp(w io.Writer) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(cache, "ytdown")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	name := "yt-dlp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	path := filepath.Join(dir, name)

	if _, err := os.Stat(path); err == nil {
		io.WriteString(w, "checking for yt-dlp updates...\n")
		cmd := exec.Command(path, "-U")
		cmd.Stdout = w
		cmd.Stderr = w
		cmd.Run()
		return path, nil
	}

	// download yt-dlp
	url := "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp"
	if runtime.GOOS == "windows" {
		url += ".exe"
	}

	io.WriteString(w, "downloading yt-dlp...\n")
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}

	return path, nil
}

func main() {
	go func() {
		window := new(app.Window)
		window.Option(app.Title("ytdown"))
		window.Option(app.Size(unit.Dp(900), unit.Dp(600)))
		err := run(window)
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}
