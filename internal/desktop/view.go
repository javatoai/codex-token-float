package desktop

import (
	"math"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/javatoai/codex-token-float/internal/usage"
)

const (
	detailSidePadding  = 16
	detailColumnGap    = 14
	detailHeaderGap    = 12
	detailNumeralWidth = 9
)

func (app *counterWindow) value() string {
	if !hasTotal(app.snapshot) {
		return "—"
	}
	return app.snapshot.FormattedTotal
}

type renderedCounts struct {
	total   string
	details [detailCount]string
}

func (app *counterWindow) countsForView(c *ui.Context, window *mygo.Window, withDetails bool) renderedCounts {
	reduced := c.Preferences().ReduceMotion
	if reduced {
		app.resetAnimations()
	}
	animate := !reduced && (window == nil || window.IsVisible())
	now := c.Now()
	moving := false
	display := func(animation countAnimation, exact string) string {
		if animate {
			if value, active := animation.valueAt(now); active {
				moving = true
				return usage.FormatTokens(strconv.FormatUint(value, 10))
			}
		}
		return exact
	}

	counts := renderedCounts{total: display(app.animation, app.value())}
	if withDetails {
		for i, value := range detailValues(app.snapshot.Breakdown) {
			if value == nil {
				counts.details[i] = "—"
			} else {
				counts.details[i] = display(app.detailAnimations[i], usage.FormatTokens(*value))
			}
		}
	}
	if moving {
		c.AnimationFrame()
	}
	return counts
}

func (app *counterWindow) render(c *ui.Context) {
	counts := app.countsForView(c, app.window, app.cardView)
	if app.cardView {
		app.detailsCard(c, counts).DragWindow().ContextMenu(app.viewMenu)
		return
	}

	theme := c.Theme()
	c.Root().Background(theme.Surface)
	labelColor, valueColor := ui.Hex("#557086"), ui.Hex("#102437")
	if theme.Dark {
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
		ui.Text(c, counts.total).Width(valueWidth).TextAlign(ui.End).
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
	if m.Item("显示美元金额").Checked(app.settings.ShowUSD).Chosen() {
		app.changeSettings(func(settings *desktopSettings) { settings.ShowUSD = !settings.ShowUSD })
	}
	if m.Item(modelPriceTitle).Chosen() {
		app.showModelPrices()
	}
	m.Separator()
	for _, seconds := range refreshChoices {
		label := "每 " + strconv.Itoa(seconds) + " 秒刷新"
		if m.Item(label).Checked(app.settings.RefreshSeconds == seconds).Chosen() {
			app.changeSettings(func(settings *desktopSettings) { settings.RefreshSeconds = seconds })
		}
	}
	m.Separator()
	if m.Item("立即刷新").Chosen() {
		app.requestRefresh()
	}
}

func (app *counterWindow) renderDetails(c *ui.Context) {
	app.detailsCard(c, app.countsForView(c, app.details, true))
}

func (app *counterWindow) detailsCard(c *ui.Context, counts renderedCounts) ui.Element {
	snapshot := app.snapshot
	showUSD := app.settings.ShowUSD
	theme := c.Theme()
	c.Root().Background(theme.Surface)
	return ui.Column(c).Fill().Padding(14, detailSidePadding).Gap(8).FontSize(13).Children(func() {
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
			ui.Row(c).Justify(ui.SpaceBetween).Gap(detailHeaderGap).Children(func() {
				ui.Text(c, counts.total).FontSize(24).FontWeight(700).FontFeatures("tnum")
				if showUSD {
					ui.Text(c, apiCostValue(snapshot.APICost)).FontWeight(600).FontFeatures("tnum")
				}
			})
		})
		ui.Box(c).Height(1).Background(theme.Border)
		costs := detailCosts(snapshot.APICostBreakdown)
		tracks := []ui.Track{ui.Fr(1), ui.FitContent()}
		if showUSD {
			tracks = append(tracks, ui.FitContent())
		}
		ui.Grid(c).ColumnTracks(tracks...).GapX(detailColumnGap).GapY(4).Children(func() {
			line := func(index int, label string, indent float32) {
				cost := ""
				if showUSD {
					cost = apiCostValue(costs[index])
				}
				detailLine(c, label, cost, counts.details[index], indent)
			}
			line(detailInput, "输入总量", 0)
			line(detailCached, "命中缓存", 12)
			line(detailUncached, "未命中输入", 12)
			if hasCacheWrites(breakdown) {
				line(detailCacheWrite, "其中写入缓存", 24)
			}
			ui.Box(c).Height(0).ColumnSpan(-1)
			line(detailOutput, "输出总量", 0)
			line(detailReasoning, "其中推理", 12)
			line(detailNonReasoning, "非推理输出", 12)
		})
		if snapshot.Message != nil {
			ui.Box(c).Height(1).Background(theme.Border)
			ui.Text(c, *snapshot.Message).FontSize(12).Wrap()
		}
	})
}

func apiCostValue(cost usage.APICost) string {
	if cost.USD == nil {
		return "$—"
	}
	amount := "$" + *cost.USD
	if cost.Incomplete {
		amount += "（部分）"
	}
	return amount
}

func detailCosts(cost usage.APICostBreakdown) [detailCount]usage.APICost {
	return [detailCount]usage.APICost{
		detailInput:        cost.Input,
		detailCached:       cost.CachedInput,
		detailUncached:     cost.UncachedInput,
		detailCacheWrite:   cost.CacheWriteInput,
		detailOutput:       cost.Output,
		detailReasoning:    cost.ReasoningOutput,
		detailNonReasoning: cost.NonReasoningOutput,
	}
}

func detailLine(c *ui.Context, label, cost, tokens string, indent float32) {
	weight, color := 600, c.Theme().Text
	if indent > 0 {
		weight, color = 400, c.Theme().TextMuted
	}
	ui.Text(c, label).PaddingStart(indent).FontWeight(weight).TextColor(color)
	if cost != "" {
		ui.Text(c, cost).TextAlign(ui.End).FontWeight(weight).TextColor(color).FontFeatures("tnum")
	}
	ui.Text(c, tokens).TextAlign(ui.End).FontWeight(weight).TextColor(color).FontFeatures("tnum")
}

func hasCacheWrites(breakdown usage.Breakdown) bool {
	return breakdown.CacheWriteInputTokens != nil && *breakdown.CacheWriteInputTokens != "0"
}

func detailWidth(snapshot usage.Snapshot, showUSD bool) int {
	// Reserve room for unusually large values without widening the usual card.
	const labelWidth, headerNumeralWidth = 108, 16
	tokensWidth, costWidth := detailNumeralWidth, 0
	costs := detailCosts(snapshot.APICostBreakdown)
	for index, value := range detailValues(snapshot.Breakdown) {
		if index == detailCacheWrite && !hasCacheWrites(snapshot.Breakdown) {
			continue
		}
		if value != nil {
			tokensWidth = max(tokensWidth, detailNumeralWidth*len(usage.FormatTokens(*value)))
		}
		if showUSD {
			costWidth = max(costWidth, apiCostWidth(costs[index]))
		}
	}
	bodyWidth := labelWidth + detailColumnGap + tokensWidth
	headerWidth := headerNumeralWidth * len(snapshot.FormattedTotal)
	if showUSD {
		bodyWidth += detailColumnGap + costWidth
		headerWidth += detailHeaderGap + apiCostWidth(snapshot.APICost)
	}
	return max(detailsWidth, 2*detailSidePadding+max(bodyWidth, headerWidth))
}

func apiCostWidth(cost usage.APICost) int {
	if cost.USD == nil {
		return 22
	}
	width := detailNumeralWidth * (1 + len(*cost.USD))
	if cost.Incomplete {
		width += 52
	}
	return width
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
