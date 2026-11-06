package desktop

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
	"github.com/javatoai/codex-token-float/assets"
	"github.com/javatoai/codex-token-float/internal/usage"
)

const (
	appName      = "Codex Token Float"
	windowWidth  = 330
	windowHeight = 76
	detailsWidth = 344
	refreshEvery = 2 * time.Second
)

type counterWindow struct {
	window           *mygo.Window
	anchor           *mygo.Window
	details          *mygo.Window
	refresh          chan<- struct{}
	snapshot         usage.Snapshot
	animation        countAnimation
	detailAnimations [detailCount]countAnimation
	cardView         bool
	tapDetails       bool
	wantDetails      bool
	quitting         bool
}

func Run(root string) error {
	mygo.App.SetName(appName)
	mygo.App.SetLanguages("zh-Hans")
	var launchErr error
	mygo.App.WhenReady(func() {
		if runtime.GOOS == "darwin" {
			mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
		}
		refresh := make(chan struct{}, 1)
		app := &counterWindow{refresh: refresh}
		app.createWindow()
		_ = app.window.SetIcon(assets.Icon)

		tray, err := app.createTray()
		if err != nil {
			launchErr = fmt.Errorf("无法创建托盘：%w", err)
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{
				Type: mygo.MessageError, Title: appName, Message: "无法创建系统托盘", Detail: err.Error(),
			})
			mygo.App.Quit()
			return
		}

		app.details = mygo.NewFlyout(mygo.FlyoutOptions{
			Parent: app.window, Width: detailsWidth, Height: detailHeight(app.snapshot), Hidden: true,
			Anchor:    mygo.Rectangle{Width: windowWidth, Height: windowHeight},
			Placement: mygo.PlacementBottom, Gap: 4, Shadow: true,
			Content: ui.View(app.renderDetails),
		})
		app.window.OnClose(func(e *mygo.CloseEvent) {
			if !app.quitting {
				e.PreventDefault()
				app.hide()
			}
		})
		app.details.OnClose(func(e *mygo.CloseEvent) {
			if !app.quitting {
				e.PreventDefault()
				app.hideDetails()
			}
		})
		ctx, cancel := context.WithCancel(context.Background())
		mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
			app.quitting = true
			app.rememberPosition()
		})
		mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
			cancel()
			tray.Destroy()
		})
		app.window.Show()
		go monitor(ctx, root, refresh, func(snapshot usage.Snapshot) {
			app.window.Update(func() {
				if app.quitting {
					return
				}
				oldHeight := detailHeight(app.snapshot)
				animate := app.window.IsVisible() && !mygo.Theme.Preferences().ReduceMotion
				app.setSnapshot(snapshot, time.Now(), animate)
				app.window.SetTitle("北京时间今日 " + app.value() + " tokens")
				if height := detailHeight(snapshot); height != oldHeight {
					app.details.SetSize(detailsWidth, height)
					if app.cardView {
						app.resizeWindow(detailsWidth, height)
					}
				}
				app.details.Invalidate()
			})
		})
	})
	err := mygo.App.Run()
	if launchErr != nil {
		return launchErr
	}
	return err
}

func (app *counterWindow) createWindow() {
	options := mygo.WindowOptions{
		Title: appName, Width: windowWidth, Height: windowHeight, StateKey: "main",
		Frameless: true, Transparent: true, DisableShadow: true, AlwaysOnTop: true,
		DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
		SkipTaskbar: true, Hidden: true, Content: ui.View(app.render),
	}
	if runtime.GOOS != "windows" {
		app.window = mygo.NewWindow(options)
		// The saved bounds may be from a card; start in the compact view.
		app.resizeWindow(windowWidth, windowHeight)
		return
	}

	// On Windows, MyGO's native flyouts use an alpha-composited surface.
	// A hidden owner provides the centered position and persists its bounds.
	options.Content = ui.View(func(*ui.Context) {})
	app.anchor = mygo.NewWindow(options)
	app.window = mygo.NewFlyout(mygo.FlyoutOptions{
		Parent: app.anchor, Width: windowWidth, Height: windowHeight,
		Hidden: true, Content: ui.View(app.render),
	})
	app.window.SetAlwaysOnTop(true)
}

func (app *counterWindow) rememberPosition() {
	if app.anchor != nil && app.window.IsVisible() {
		x, y := app.window.Position()
		app.anchor.SetPosition(x, y)
	}
}

func (app *counterWindow) resizeWindow(width, height int) {
	if app.anchor != nil {
		// SetSize places a flyout at its owner again. Move the owner to
		// the dragged position first, then let MyGO fit it to the display.
		app.rememberPosition()
		app.window.SetSize(width, height)
		return
	}
	b := app.window.Bounds()
	work := mygo.Screen.DisplayMatching(b).WorkArea
	b.Width, b.Height = width, height
	if work.Width > 0 && work.Height > 0 {
		b.X = max(work.X, min(b.X, work.X+work.Width-width))
		b.Y = max(work.Y, min(b.Y, work.Y+work.Height-height))
	}
	app.window.SetBounds(b)
}

func (app *counterWindow) setCardView(card bool) {
	app.window.Update(func() {
		if app.quitting || app.cardView == card {
			return
		}
		app.cardView = card
		app.hideDetails()
		if card {
			app.resizeWindow(detailsWidth, detailHeight(app.snapshot))
		} else {
			app.resizeWindow(windowWidth, windowHeight)
		}
	})
}

func (app *counterWindow) hideDetails() {
	app.tapDetails, app.wantDetails = false, false
	app.details.Hide()
}

func (app *counterWindow) show() {
	app.window.Show()
	app.window.Focus()
	app.window.Invalidate()
}

func (app *counterWindow) hide() {
	app.resetAnimations()
	app.rememberPosition()
	app.hideDetails()
	app.window.Hide()
}

func (app *counterWindow) requestRefresh() {
	select {
	case app.refresh <- struct{}{}:
	default:
	}
}

func (app *counterWindow) createTray() (*mygo.Tray, error) {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "显示悬浮框", Click: func(*mygo.MenuItem, *mygo.Window) { app.show() }},
		{Label: "隐藏悬浮框", Click: func(*mygo.MenuItem, *mygo.Window) { app.hide() }},
		{Label: "立即刷新", Click: func(*mygo.MenuItem, *mygo.Window) { app.requestRefresh() }},
		mygo.Separator(),
		{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{Icon: assets.Icon, ToolTip: "Codex 今日 Token", Menu: menu})
	if err == nil {
		tray.OnClick(app.show)
	}
	return tray, err
}

func monitor(ctx context.Context, root string, refresh <-chan struct{}, publish func(usage.Snapshot)) {
	index := usage.NewIndex(root)
	ticker := time.NewTicker(refreshEvery)
	defer ticker.Stop()
	publish(index.Scan(time.Now(), false))
	for {
		rebuild := false
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-refresh:
			rebuild = true
		}
		publish(index.Scan(time.Now(), rebuild))
	}
}
