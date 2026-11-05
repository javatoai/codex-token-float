package desktop

import (
	"math"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/javatoai/codex-token-float/internal/usage"
)

func (app *counterWindow) value() string {
	if !hasTotal(app.snapshot) {
		return "—"
	}
	return app.snapshot.FormattedTotal
}

func (app *counterWindow) displayedValue(c *ui.Context, window *mygo.Window) string {
	value, moving := app.animation.valueAt(c.Now())
	if !moving {
		return app.value()
	}
	if c.Preferences().ReduceMotion {
		app.animation = countAnimation{}
		return app.value()
	}
	if window != nil && !window.IsVisible() {
		return app.value()
	}
	c.AnimationFrame()
	return usage.FormatTokens(strconv.FormatUint(value, 10))
}

func (app *counterWindow) render(c *ui.Context) {
	value := app.displayedValue(c, app.window)
	if app.cardView {
		app.detailsCard(c, value).DragWindow().ContextMenu(app.viewMenu)
		return
	}

	c.Root().Background(ui.Transparent)
	labelColor, valueColor := ui.Hex("#557086"), ui.Hex("#102437")
	if c.Theme().Dark {
		labelColor, valueColor = ui.Hex("#9db3c7"), ui.Hex("#f5fbff")
	}
	const labelSize, valueWeight, digitFeatures = 13, 740, "tnum"
	target := app.value()
	fontSize := max(14, 23-0.75*float32(max(0, len(target)-12)))
	// Reserve the target width so a new digit or comma does not move the labels mid-animation.
	width, _ := c.MeasureText(0, ui.Span{Text: target, Size: fontSize, Weight: valueWeight, Features: digitFeatures})
	valueWidth := float32(math.Ceil(float64(width)))
	// Centering the line boxes leaves the smaller glyphs visibly high.
	labelOffset := (fontSize - labelSize) * 0.3
	row := ui.Row(c).Fill().Center().Gap(8).DragWindow().ContextMenu(app.viewMenu).Children(func() {
		today := ui.ButtonBase(c).Padding(10, 4).Cursor(ui.CursorPointer).
			Description("展开或收起 Token 明细").Children(func() {
			ui.Text(c, "今日").FontSize(labelSize).FontWeight(600).TextColor(labelColor).Translate(0, labelOffset)
		})
		if today.Clicked() {
			app.tapDetails = !app.tapDetails
		}
		ui.Text(c, value).Width(valueWidth).TextAlign(ui.End).
			FontSize(fontSize).FontWeight(valueWeight).FontFeatures(digitFeatures).TextColor(valueColor)
		ui.Text(c, "tokens").FontSize(labelSize).FontWeight(600).TextColor(labelColor).Translate(0, labelOffset)
	})

	// Use the toolkit's hover delay and dismissal rules, then show the
	// details in a native flyout with room outside the short counter window.
	wantDetails := app.tapDetails
	ui.TooltipBase(c, row, func(ui.Element) { wantDetails = true })
	if app.details != nil && wantDetails != app.wantDetails {
		app.wantDetails = wantDetails
		app.window.Update(func() {
			if app.quitting {
				return
			}
			if app.wantDetails && !app.cardView && app.window.IsVisible() {
				app.details.ShowInactive()
				app.details.Invalidate()
			} else {
				app.details.Hide()
			}
		})
	}
}

func (app *counterWindow) viewMenu(m *ui.Menu) {
	if m.Item("卡片视图").Checked(app.cardView).Chosen() {
		app.setCardView(true)
	}
	if m.Item("弹框视图").Checked(!app.cardView).Chosen() {
		app.setCardView(false)
	}
	m.Separator()
	if m.Item("立即刷新").Chosen() {
		app.requestRefresh()
	}
}

func (app *counterWindow) renderDetails(c *ui.Context) {
	app.detailsCard(c, app.displayedValue(c, app.details))
}

func (app *counterWindow) detailsCard(c *ui.Context, value string) ui.Element {
	snapshot := app.snapshot
	theme := c.Theme()
	c.Root().Background(theme.Surface)
	return ui.Column(c).Fill().Padding(14, 16).Gap(8).FontSize(13).Children(func() {
		if snapshot.Status == "" {
			ui.Text(c, "正在读取 Codex 用量…")
			return
		}

		breakdown := snapshot.Breakdown
		rate := "—"
		if breakdown.CacheHitRatePercent != nil {
			rate = *breakdown.CacheHitRatePercent + "%"
		}
		ui.Column(c).Gap(3).Children(func() {
			ui.Row(c).Justify(ui.SpaceBetween).Gap(8).FontSize(12).Children(func() {
				ui.Text(c, "今日总 Token").FontWeight(600)
				ui.Text(c, "缓存命中率 "+rate).FontFeatures("tnum").TextColor(theme.TextMuted)
			})
			ui.Text(c, value).FontSize(24).FontWeight(700).FontFeatures("tnum")
		})
		ui.Box(c).Height(1).Background(theme.Border)
		ui.Column(c).Gap(4).Children(func() {
			detailLine(c, "输入总量", breakdown.InputTokens, 0)
			detailLine(c, "命中缓存", breakdown.CachedInputTokens, 12)
			detailLine(c, "未命中输入", breakdown.UncachedInputTokens, 12)
			if hasCacheWrites(breakdown) {
				detailLine(c, "其中写入缓存", breakdown.CacheWriteInputTokens, 24)
			}
		})
		ui.Column(c).Gap(4).Children(func() {
			detailLine(c, "输出总量", breakdown.OutputTokens, 0)
			detailLine(c, "其中推理", breakdown.ReasoningOutputTokens, 12)
			detailLine(c, "非推理输出", breakdown.NonReasoningOutputTokens, 12)
		})
		if snapshot.Message != nil {
			ui.Box(c).Height(1).Background(theme.Border)
			ui.Text(c, *snapshot.Message).FontSize(12).Wrap()
		}
	})
}

func detailLine(c *ui.Context, label string, count *string, indent float32) {
	value := "—"
	if count != nil {
		value = usage.FormatTokens(*count)
	}
	row := ui.Row(c).Justify(ui.SpaceBetween).Gap(8).Padding(0, 0, 0, indent)
	if indent == 0 {
		row.FontWeight(600)
	} else {
		row.TextColor(c.Theme().TextMuted)
	}
	row.Children(func() {
		ui.Text(c, label)
		ui.Text(c, value).FontFeatures("tnum")
	})
}

func hasCacheWrites(breakdown usage.Breakdown) bool {
	return breakdown.CacheWriteInputTokens != nil && *breakdown.CacheWriteInputTokens != "0"
}

func detailHeight(snapshot usage.Snapshot) int {
	if snapshot.Status == "" {
		return 52
	}
	height := 234
	if hasCacheWrites(snapshot.Breakdown) {
		height += 22
	}
	if snapshot.Message != nil {
		height += 32
	}
	return height
}
