package desktop

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/javatoai/codex-token-float/internal/usage"
)

const (
	modelPriceTitle  = "模型单价"
	modelPriceWidth  = 480
	modelPriceHeight = 360
)

func (app *counterWindow) showModelPrices() {
	app.window.Update(func() {
		if app.quitting {
			return
		}
		if app.priceWindow != nil {
			app.priceWindow.Show()
			app.priceWindow.Focus()
			return
		}
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: modelPriceTitle, Width: modelPriceWidth, Height: modelPriceHeight, UseContentSize: true,
			Parent: app.window, SkipTaskbar: true, DisableResize: true,
			DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
			Content: ui.View(renderModelPrices),
		})
		app.priceWindow = window
		window.OnClosed(func() { app.priceWindow = nil })
		window.Focus()
	})
}

func renderModelPrices(c *ui.Context) {
	theme := c.Theme()
	c.Root().Background(theme.Surface)
	ui.Column(c).Fill().Padding(16, 20).Gap(10).FontSize(13).Children(func() {
		ui.Text(c, "美元／百万 Token").FontSize(12).TextColor(theme.TextMuted)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Grid(c).ColumnTracks(ui.Fr(1), ui.FitContent(), ui.FitContent(), ui.FitContent()).GapX(16).GapY(8).Children(func() {
				ui.Text(c, "模型").FontWeight(600)
				for _, label := range []string{"未缓存输入", "缓存命中", "输出"} {
					ui.Text(c, label).TextAlign(ui.End).FontWeight(600)
				}
				ui.Box(c).Height(1).Background(theme.Border).ColumnSpan(-1)
				for _, price := range usage.ModelPrices() {
					name := price.Name
					if price.Promotional {
						name += "（优惠价）"
					}
					ui.Text(c, name)
					for _, cents := range []uint64{price.UncachedInputCents, price.CachedInputCents, price.OutputCents} {
						ui.Text(c, fmt.Sprintf("%d.%02d", cents/100, cents%100)).TextAlign(ui.End).FontFeatures("tnum")
					}
				}
			})
		})
	})
}
