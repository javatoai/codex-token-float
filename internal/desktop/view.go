package desktop

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/javatoai/codex-token-float/internal/usage"
)

func (app *counterWindow) value() string {
	if app.snapshot.Status == "" || app.snapshot.Status == usage.MissingDirectory {
		return "—"
	}
	return app.snapshot.FormattedTotal
}

func (app *counterWindow) render(c *ui.Context) {
	c.Root().Background(ui.Transparent)
	labelColor, valueColor := ui.Hex("#557086"), ui.Hex("#102437")
	if c.Theme().Dark {
		labelColor, valueColor = ui.Hex("#9db3c7"), ui.Hex("#f5fbff")
	}
	value := app.value()
	fontSize := max(14, 23-0.75*float32(max(0, len(value)-12)))
	row := ui.Row(c).Fill().Center().Gap(8).DragWindow().Children(func() {
		ui.Text(c, "今日").FontSize(13).FontWeight(600).TextColor(labelColor)
		ui.Text(c, value).FontSize(fontSize).FontWeight(740).FontFeatures("tnum").TextColor(valueColor)
		ui.Text(c, "tokens").FontSize(13).FontWeight(600).TextColor(labelColor)
	})

	// Use the toolkit's hover delay and dismissal rules, then show the
	// details in a native flyout with room outside the short counter window.
	wantDetails := false
	ui.TooltipBase(c, row, func(ui.Element) { wantDetails = true })
	if app.details != nil && wantDetails != app.wantDetails {
		app.wantDetails = wantDetails
		app.window.Update(func() {
			if app.quitting {
				return
			}
			if app.wantDetails && app.window.IsVisible() {
				app.details.Show()
			} else {
				app.details.Hide()
			}
		})
	}
}

func (app *counterWindow) renderDetails(c *ui.Context) {
	snapshot := app.snapshot
	c.Root().Background(c.Theme().Surface)
	ui.Column(c).Fill().Padding(12).Gap(4).FontSize(12).Children(func() {
		if snapshot.Status == "" {
			ui.Text(c, "正在读取 Codex 用量…")
			return
		}
		ui.Text(c, fmt.Sprintf("北京时间 %s · 更新于 %s", snapshot.Date, snapshot.UpdatedAt))
		ui.Text(c, "精确总数  "+snapshot.TotalTokens+" tokens")
		ui.Text(c, fmt.Sprintf("日志文件 %d · 用量事件 %d", snapshot.FileCount, snapshot.EventCount))
		if snapshot.Message != nil {
			ui.Text(c, *snapshot.Message).Wrap()
		}
	})
}
