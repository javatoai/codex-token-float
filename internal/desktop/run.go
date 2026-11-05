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
	refreshEvery = 2 * time.Second
)

type counterWindow struct {
	window      *mygo.Window
	anchor      *mygo.Window
	details     *mygo.Window
	snapshot    usage.Snapshot
	wantDetails bool
	quitting    bool
}

func Run(root string) error {
	mygo.App.SetName(appName)
	mygo.App.SetLanguages("zh-Hans")
	var launchErr error
	mygo.App.WhenReady(func() {
		if runtime.GOOS == "darwin" {
			mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
		}
		app := &counterWindow{}
		app.createWindow()
		_ = app.window.SetIcon(assets.Icon)

		refresh := make(chan struct{}, 1)
		tray, err := app.createTray(refresh)
		if err != nil {
			launchErr = fmt.Errorf("无法创建托盘：%w", err)
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{
				Type: mygo.MessageError, Title: appName, Message: "无法创建系统托盘", Detail: err.Error(),
			})
			mygo.App.Quit()
			return
		}

		app.details = mygo.NewFlyout(mygo.FlyoutOptions{
			Parent: app.window, Width: windowWidth, Height: 126, Hidden: true,
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
				app.details.Hide()
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
				app.snapshot = snapshot
				app.window.SetTitle("北京时间今日 " + app.value() + " tokens")
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

func (app *counterWindow) show() {
	app.window.Show()
	app.window.Focus()
}

func (app *counterWindow) hide() {
	app.rememberPosition()
	app.wantDetails = false
	app.details.Hide()
	app.window.Hide()
}

func (app *counterWindow) createTray(refresh chan<- struct{}) (*mygo.Tray, error) {
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "显示悬浮框", Click: func(*mygo.MenuItem, *mygo.Window) { app.show() }},
		{Label: "隐藏悬浮框", Click: func(*mygo.MenuItem, *mygo.Window) { app.hide() }},
		{Label: "立即刷新", Click: func(*mygo.MenuItem, *mygo.Window) {
			select {
			case refresh <- struct{}{}:
			default:
			}
		}},
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
