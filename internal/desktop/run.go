package desktop

import (
	"context"
	"fmt"
	"path/filepath"
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
)

type counterWindow struct {
	window           *mygo.Window
	anchor           *mygo.Window
	details          *mygo.Window
	priceWindow      *mygo.Window
	refresh          chan<- struct{}
	intervalChanges  chan time.Duration
	settingsPath     string
	settings         desktopSettings
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
	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return fmt.Errorf("无法获取应用数据目录：%w", err)
	}
	release, err := tryInstanceLock(filepath.Join(dataDir, "instance.lock"))
	if err != nil {
		return fmt.Errorf("无法锁定应用实例：%w", err)
	}
	if release == nil {
		return nil
	}
	defer release()

	settingsPath := filepath.Join(dataDir, "settings.json")
	settings := loadSettings(settingsPath)
	mygo.App.SetLanguages("zh-Hans")
	var launchErr error
	mygo.App.WhenReady(func() {
		if runtime.GOOS == "darwin" {
			mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
		}
		refresh := make(chan struct{}, 1)
		intervalChanges := make(chan time.Duration, 1)
		app := &counterWindow{
			refresh: refresh, intervalChanges: intervalChanges, cardView: true,
			settingsPath: settingsPath, settings: settings,
		}
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

		width, height := app.detailSize()
		app.details = mygo.NewFlyout(mygo.FlyoutOptions{
			Parent: app.window, Width: width, Height: height, Hidden: true,
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
		go monitor(ctx, root, settings.interval(), intervalChanges, refresh, func(snapshot usage.Snapshot) {
			app.window.Update(func() {
				if app.quitting {
					return
				}
				oldWidth, oldHeight := app.detailSize()
				animate := app.window.IsVisible() && !mygo.Theme.Preferences().ReduceMotion
				app.setSnapshot(snapshot, time.Now(), animate)
				app.window.SetTitle("北京时间今日 " + app.value() + " tokens")
				app.updateDetails(oldWidth, oldHeight)
			})
		})
	})
	err = mygo.App.Run()
	if launchErr != nil {
		return launchErr
	}
	return err
}

func (app *counterWindow) windowSize() (int, int) {
	if app.cardView {
		return app.detailSize()
	}
	return windowWidth, windowHeight
}

func (app *counterWindow) detailSize() (int, int) {
	return detailWidth(app.snapshot, app.settings.ShowUSD), detailHeight(app.snapshot)
}

func (app *counterWindow) updateDetails(oldWidth, oldHeight int) {
	width, height := app.detailSize()
	if width != oldWidth || height != oldHeight {
		app.details.SetSize(width, height)
		if app.cardView {
			app.resizeWindow(width, height)
		}
	}
	app.details.Invalidate()
}

func (app *counterWindow) createWindow() {
	width, height := app.windowSize()
	options := mygo.WindowOptions{
		Title: appName, Width: width, Height: height, StateKey: "main",
		Frameless: true, Transparent: true, DisableShadow: true, AlwaysOnTop: true,
		DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
		SkipTaskbar: true, Hidden: true, Content: ui.View(app.render),
	}
	if runtime.GOOS != "windows" {
		app.window = mygo.NewWindow(options)
		app.resizeWindow(width, height)
		return
	}

	// On Windows, MyGO's native flyouts use an alpha-composited surface.
	// A hidden owner provides the centered position and persists its bounds.
	options.Content = ui.View(func(*ui.Context) {})
	app.anchor = mygo.NewWindow(options)
	app.window = mygo.NewFlyout(mygo.FlyoutOptions{
		Parent: app.anchor, Width: width, Height: height,
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
		app.resizeWindow(app.windowSize())
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

func (app *counterWindow) changeSettings(change func(*desktopSettings)) {
	app.window.Update(func() {
		if app.quitting {
			return
		}
		previous := app.settings
		next := previous
		change(&next)
		if next == previous {
			return
		}
		if err := saveSettings(app.settingsPath, next); err != nil {
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{
				Type: mygo.MessageWarning, Title: appName, Message: "无法保存设置", Detail: err.Error(),
			})
			return
		}
		oldWidth, oldHeight := app.detailSize()
		app.settings = next
		if next.RefreshSeconds != previous.RefreshSeconds {
			// A newer choice supersedes an interval queued during a scan.
			select {
			case <-app.intervalChanges:
			default:
			}
			app.intervalChanges <- next.interval()
		}
		if next.ShowUSD != previous.ShowUSD {
			app.updateDetails(oldWidth, oldHeight)
		}
	})
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

func monitor(ctx context.Context, root string, interval time.Duration, intervalChanges <-chan time.Duration, refresh <-chan struct{}, publish func(usage.Snapshot)) {
	index := usage.NewIndex(root)
	publish(index.Scan(time.Now(), false))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		rebuild := false
		select {
		case <-ctx.Done():
			return
		case interval := <-intervalChanges:
			ticker.Reset(interval)
			continue
		case <-ticker.C:
		case <-refresh:
			rebuild = true
		}
		publish(index.Scan(time.Now(), rebuild))
	}
}
