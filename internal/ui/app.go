package ui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/go-toast/toast"

	"bambupresence/internal/bambu"
	"bambupresence/internal/config"
	"bambupresence/internal/discord"
	"bambupresence/internal/history"
)

type App struct {
	th     *material.Theme
	window *app.Window

	mu            sync.Mutex
	cfg           config.Config
	bambuClient   *bambu.Client
	discordClient *discord.Client
	historyMgr    *history.Manager
	lastState     bambu.PrinterState
	isConnecting  bool
	prevGcode     string
	lastErrorKey  string
	logLines      []string

	saveBtn       widget.Clickable
	connectBtn    widget.Clickable
	disconnectBtn widget.Clickable

	leftList widget.List
	logList  widget.List

	ipEd       widget.Editor
	accessEd   widget.Editor
	serialEd   widget.Editor
	clientIDEd widget.Editor

	chkFilename    widget.Bool
	chkLayers      widget.Bool
	chkTime        widget.Bool
	chkTemps       widget.Bool
	chkFilament    widget.Bool
	chkErrors      widget.Bool
	chkHideIdle    widget.Bool
	chkAutoConnect widget.Bool
}

func NewApp(cfg config.Config) *App {
	a := &App{
		th:  newTheme(),
		cfg: cfg,
	}

	a.leftList.Axis = layout.Vertical
	a.logList.Axis = layout.Vertical

	a.ipEd.SingleLine = true
	a.accessEd.SingleLine = true
	a.accessEd.Mask = '*'
	a.serialEd.SingleLine = true
	a.clientIDEd.SingleLine = true

	a.window = new(app.Window)
	a.window.Option(
		app.Title("BambuPresence"),
		app.Size(unit.Dp(860), unit.Dp(560)),
	)

	a.historyMgr = history.NewManager(history.DefaultHistoryPath, nil)
	a.loadConfigToUI()
	return a
}

func (a *App) Run() {
	a.appendLog("BambuPresence ready.")
	if a.cfg.AutoConnect && a.cfg.IP != "" && a.cfg.AccessCode != "" && a.cfg.Serial != "" {
		go func() {
			time.Sleep(250 * time.Millisecond)
			a.connect()
		}()
	}

	go func() {
		if err := a.loop(); err != nil {
			fmt.Fprintf(os.Stderr, "UI error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func (a *App) invalidate() {
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) loop() error {
	var ops op.Ops
	for {
		switch e := a.window.Event().(type) {
		case app.DestroyEvent:
			a.disconnect()
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.handleEvents(gtx)
			paint.Fill(gtx.Ops, colorBg)
			a.layoutRoot(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (a *App) handleEvents(gtx layout.Context) {
	if a.saveBtn.Clicked(gtx) {
		a.saveSettingsFromUI()
	}
	if a.connectBtn.Clicked(gtx) {
		go a.connect()
	}
	if a.disconnectBtn.Clicked(gtx) {
		go a.disconnect()
	}
}

func (a *App) layoutRoot(gtx layout.Context) layout.Dimensions {
	return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(300))
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(320))
				return a.layoutLeftPanel(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(14)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.layoutRightPanel(gtx)
			}),
		)
	})
}

func (a *App) layoutLeftPanel(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	connected := a.bambuClient != nil
	a.mu.Unlock()

	items := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			lbl := material.H6(a.th, "Settings")
			lbl.Font.Weight = 700
			return lbl.Layout(gtx)
		},
		a.inputField("Printer IP", &a.ipEd, "192.168.1.50"),
		a.inputField("Access Code", &a.accessEd, "LAN Access Code"),
		a.inputField("Serial Number", &a.serialEd, "01P00A..."),
		a.inputField("Discord Client ID", &a.clientIDEd, "18-digit App ID"),
		material.CheckBox(a.th, &a.chkFilename, "Show File Name").Layout,
		material.CheckBox(a.th, &a.chkLayers, "Show Layers").Layout,
		material.CheckBox(a.th, &a.chkTime, "Show Remaining Time").Layout,
		material.CheckBox(a.th, &a.chkTemps, "Show Temperatures").Layout,
		material.CheckBox(a.th, &a.chkFilament, "Show Active Filament").Layout,
		material.CheckBox(a.th, &a.chkErrors, "Show Errors & Alerts").Layout,
		material.CheckBox(a.th, &a.chkHideIdle, "Hide When Idle").Layout,
		material.CheckBox(a.th, &a.chkAutoConnect, "Auto-Connect").Layout,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.actionButton(gtx, &a.saveBtn, "Save Settings", colorOverlay, true)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.actionButton(gtx, &a.connectBtn, "Connect", colorPrimary, !connected)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return a.actionButton(gtx, &a.disconnectBtn, "Disconnect", colorDanger, connected)
				}),
			)
		},
	}

	return fillSurface(gtx, colorSurface, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return material.List(a.th, &a.leftList).Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, items[i])
			})
		})
	})
}

func (a *App) layoutRightPanel(gtx layout.Context) layout.Dimensions {
	a.mu.Lock()
	st := a.lastState
	bc := a.bambuClient
	connecting := a.isConnecting
	logs := make([]string, len(a.logLines))
	copy(logs, a.logLines)
	a.mu.Unlock()

	dotCol := colorMuted
	statusText := "Disconnected"
	if bc != nil {
		if st.Connected {
			if len(st.Errors) > 0 {
				dotCol = colorDanger
				statusText = fmt.Sprintf("Error: %s", st.Errors[0].Message)
			} else {
				dotCol = colorPrimary
				statusText = nonEmpty(st.GcodeState, "Connected")
			}
		} else if connecting {
			dotCol = colorWarning
			statusText = "Connecting..."
		} else {
			dotCol = colorWarning
			statusText = "Reconnecting..."
		}
	}

	layerStr := "--"
	if st.TotalLayers > 0 {
		layerStr = fmt.Sprintf("%d / %d", st.Layer, st.TotalLayers)
	}
	remainStr := "--"
	if st.RemainingMins > 0 {
		remainStr = fmt.Sprintf("%dm", st.RemainingMins)
	}
	tempStr := fmt.Sprintf("Nozzle %.0fC  |  Bed %.0fC", st.NozzleTemp, st.BedTemp)
	if st.ChamberTemp > 0 {
		tempStr += fmt.Sprintf("  |  Chamber %.0fC", st.ChamberTemp)
	}
	filStr := nonEmpty(st.ActiveFilamentLabel(), "--")

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fillSurface(gtx, colorSurface, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return drawDot(gtx, dotCol, 10)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									lbl := material.H6(a.th, statusText)
									lbl.Font.Weight = 700
									return lbl.Layout(gtx)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Body2(a.th, fmt.Sprintf("File: %s", nonEmpty(st.SubtaskName, "--"))).Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return material.Body2(a.th, fmt.Sprintf("Layer: %s   |   Remaining: %s   |   Filament: %s", layerStr, remainStr, filStr)).Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body2(a.th, tempStr)
							lbl.Color = colorMuted
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return a.progressBar(gtx, st.Percent)
						}),
					)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return fillSurface(gtx, colorSurface, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body1(a.th, "Logs")
							lbl.Font.Weight = 700
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return fillSurface(gtx, colorBg, func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return material.List(a.th, &a.logList).Layout(gtx, len(logs), func(gtx layout.Context, i int) layout.Dimensions {
										return material.Caption(a.th, logs[i]).Layout(gtx)
									})
								})
							})
						}),
					)
				})
			})
		}),
	)
}

func (a *App) loadConfigToUI() {
	c := a.cfg
	a.ipEd.SetText(c.IP)
	a.accessEd.SetText(c.AccessCode)
	a.serialEd.SetText(c.Serial)
	a.clientIDEd.SetText(c.ClientID)
	a.chkFilename.Value = c.ShowFilename
	a.chkLayers.Value = c.ShowLayers
	a.chkTime.Value = c.ShowTime
	a.chkTemps.Value = c.ShowTemps
	a.chkFilament.Value = c.ShowFilament
	a.chkErrors.Value = c.ShowErrors
	a.chkHideIdle.Value = c.HideWhenIdle
	a.chkAutoConnect.Value = c.AutoConnect
}

func (a *App) saveSettingsFromUI() {
	a.mu.Lock()
	newCfg := a.cfg
	dc := a.discordClient
	a.mu.Unlock()

	newCfg.IP = strings.TrimSpace(a.ipEd.Text())
	newCfg.AccessCode = strings.TrimSpace(a.accessEd.Text())
	newCfg.Serial = strings.TrimSpace(a.serialEd.Text())
	newCfg.ClientID = strings.TrimSpace(a.clientIDEd.Text())
	newCfg.ShowFilename = a.chkFilename.Value
	newCfg.ShowLayers = a.chkLayers.Value
	newCfg.ShowTime = a.chkTime.Value
	newCfg.ShowTemps = a.chkTemps.Value
	newCfg.ShowFilament = a.chkFilament.Value
	newCfg.ShowErrors = a.chkErrors.Value
	newCfg.HideWhenIdle = a.chkHideIdle.Value
	newCfg.AutoConnect = a.chkAutoConnect.Value

	a.mu.Lock()
	a.cfg = newCfg
	a.mu.Unlock()

	if err := config.Save(config.DefaultConfigPath, newCfg); err != nil {
		a.appendLog(fmt.Sprintf("Failed to save config: %v", err))
		return
	}
	if dc != nil {
		dc.UpdateConfig(newCfg)
	}
	a.appendLog("Settings saved.")
	a.invalidate()
}

func (a *App) connect() {
	a.disconnect()
	a.saveSettingsFromUI()

	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()

	if cfg.IP == "" || cfg.AccessCode == "" || cfg.Serial == "" {
		a.appendLog("Error: Please enter Printer IP, Access Code, and Serial Number.")
		return
	}

	dc := discord.NewClient(cfg.ClientID, cfg, a.appendLog)
	bc := bambu.NewClient(cfg.IP, cfg.AccessCode, cfg.Serial, a.onPrinterUpdate, a.appendLog)

	a.mu.Lock()
	a.discordClient = dc
	a.bambuClient = bc
	a.isConnecting = true
	a.mu.Unlock()
	a.invalidate()

	_ = dc.Connect()
	_ = bc.Connect()

	a.mu.Lock()
	a.isConnecting = false
	a.mu.Unlock()
	a.invalidate()
}

func (a *App) disconnect() {
	a.mu.Lock()
	bc := a.bambuClient
	dc := a.discordClient
	a.bambuClient = nil
	a.discordClient = nil
	a.isConnecting = false
	a.lastState.Connected = false
	a.mu.Unlock()

	if bc != nil {
		bc.Disconnect()
	}
	if dc != nil {
		dc.Disconnect()
	}
	a.invalidate()
}

func (a *App) onPrinterUpdate(st bambu.PrinterState) {
	a.historyMgr.Observe(st)
	a.checkNotifications(st)

	a.mu.Lock()
	a.lastState = st
	dc := a.discordClient
	a.mu.Unlock()

	if dc != nil {
		dc.UpdatePresence(st)
	}
	a.invalidate()
}

func (a *App) checkNotifications(st bambu.PrinterState) {
	a.mu.Lock()
	cfg := a.cfg
	prev := a.prevGcode
	prevErr := a.lastErrorKey
	curr := strings.ToUpper(st.GcodeState)
	a.prevGcode = curr

	currErr := ""
	if len(st.Errors) > 0 {
		currErr = st.Errors[0].Code
	}
	a.lastErrorKey = currErr
	a.mu.Unlock()

	if cfg.NotifyFinish && prev == "RUNNING" && curr == "FINISH" {
		sendDesktopNotification("Print Completed", fmt.Sprintf("%s finished.", nonEmpty(st.SubtaskName, "Your print")))
	}
	if cfg.NotifyPause && prev == "RUNNING" && curr == "PAUSE" {
		sendDesktopNotification("Print Paused", nonEmpty(st.StageText, "Printer paused"))
	}
	if cfg.NotifyError && currErr != "" && currErr != prevErr {
		sendDesktopNotification("Bambu Printer Error", st.Errors[0].Message)
	}
}

func sendDesktopNotification(title, message string) {
	n := toast.Notification{
		AppID:   "BambuPresence",
		Title:   title,
		Message: message,
	}
	go func() {
		_ = n.Push()
	}()
}

func (a *App) appendLog(msg string) {
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	a.mu.Lock()
	a.logLines = append(a.logLines, line)
	if len(a.logLines) > 200 {
		a.logLines = a.logLines[len(a.logLines)-200:]
	}
	a.mu.Unlock()
	a.invalidate()
}

func (a *App) inputField(label string, ed *widget.Editor, hint string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Caption(a.th, label)
				lbl.Color = colorMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fillSurface(gtx, colorOverlay, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(7)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						e := material.Editor(a.th, ed, hint)
						e.Color = colorFg
						e.HintColor = colorMuted
						return e.Layout(gtx)
					})
				})
			}),
		)
	}
}

func (a *App) progressBar(gtx layout.Context, pct int) layout.Dimensions {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(unit.Dp(10))
			w := gtx.Constraints.Max.X
			bgRect := image.Rect(0, 0, w, h)
			paint.FillShape(gtx.Ops, colorOverlay, clip.UniformRRect(bgRect, h/2).Op(gtx.Ops))
			fillW := (w * pct) / 100
			if fillW > 0 {
				fgRect := image.Rect(0, 0, fillW, h)
				paint.FillShape(gtx.Ops, colorPrimary, clip.UniformRRect(fgRect, h/2).Op(gtx.Ops))
			}
			return layout.Dimensions{Size: image.Pt(w, h)}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Body2(a.th, fmt.Sprintf("%d%%", pct))
			lbl.Font.Weight = 700
			return lbl.Layout(gtx)
		}),
	)
}

func (a *App) actionButton(gtx layout.Context, clk *widget.Clickable, label string, bg color.NRGBA, enabled bool) layout.Dimensions {
	if !enabled {
		gtx = gtx.Disabled()
		bg = colorOverlay
	}
	btn := material.Button(a.th, clk, label)
	btn.Background = bg
	btn.Color = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	if !enabled {
		btn.Color = colorMuted
	}
	btn.CornerRadius = unit.Dp(5)
	btn.Inset = layout.Inset{Top: unit.Dp(7), Bottom: unit.Dp(7), Left: unit.Dp(12), Right: unit.Dp(12)}
	return btn.Layout(gtx)
}

func fillSurface(gtx layout.Context, bg color.NRGBA, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()

	rect := image.Rect(0, 0, dims.Size.X, dims.Size.Y)
	paint.FillShape(gtx.Ops, bg, clip.UniformRRect(rect, gtx.Dp(unit.Dp(6))).Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

func drawDot(gtx layout.Context, col color.NRGBA, dp int) layout.Dimensions {
	sz := gtx.Dp(unit.Dp(dp))
	rect := image.Rect(0, 0, sz, sz)
	paint.FillShape(gtx.Ops, col, clip.Ellipse(rect).Op(gtx.Ops))
	return layout.Dimensions{Size: image.Pt(sz, sz)}
}

func nonEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
