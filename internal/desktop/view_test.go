package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/javatoai/codex-token-float/internal/usage"
)

func TestDetailsFitAtHighDPI(t *testing.T) {
	message := "部分用量缺少分类明细"
	app := &counterWindow{snapshot: usage.Snapshot{
		Status:         usage.PartialError,
		Date:           "2026-11-05",
		UpdatedAt:      "23:59:59",
		FormattedTotal: usage.FormatTokens("18446744073709551615"),
		Message:        &message,
		Breakdown: usage.Breakdown{
			InputTokens:              new("18446744073709541615"),
			CachedInputTokens:        new("17791978310692853637"),
			UncachedInputTokens:      new("654765763016687978"),
			CacheWriteInputTokens:    new("10000"),
			OutputTokens:             new("10000"),
			ReasoningOutputTokens:    new("3000"),
			NonReasoningOutputTokens: new("7000"),
			CacheHitRatePercent:      new("96.45"),
		},
	}}
	height := detailHeight(app.snapshot)
	preview := ui.NewTester(app.renderDetails, detailsWidth, height)
	preview.SetScale(1.5)
	preview.SetDark(true)
	if got := preview.Image().Bounds().Dx(); got != detailsWidth*3/2 {
		t.Fatalf("render width at 150%% scaling = %d", got)
	}
	for _, label := range []string{
		app.snapshot.FormattedTotal, "缓存命中率 96.45%", "输入总量",
		"命中缓存", "未命中输入", "其中写入缓存", "输出总量", "其中推理",
		"非推理输出", "7000",
		message,
	} {
		rect, ok := preview.Find(label)
		if !ok || rect.X < 0 || rect.Y < 0 || rect.X+rect.W > detailsWidth || rect.Y+rect.H > float32(height) {
			t.Errorf("%q not visible within the details window: %+v", label, rect)
		}
	}
	if preview.HasText("北京时间") || preview.HasText(app.snapshot.UpdatedAt) {
		t.Fatalf("details must omit the date and refresh time: %v", preview.Texts())
	}

	app.snapshot.Breakdown.CacheWriteInputTokens = new("0")
	app.snapshot.Breakdown.CacheHitRatePercent = nil
	preview.SetSize(detailsWidth, detailHeight(app.snapshot))
	if preview.HasText("其中写入缓存") || !preview.HasText("缓存命中率 —") {
		t.Fatalf("unexpected rows with zero cache writes and unknown rate: %v", preview.Texts())
	}
}

func TestDetailViewsAnimateWhenTotalIsUnchanged(t *testing.T) {
	for _, card := range []bool{false, true} {
		name := "hover"
		if card {
			name = "card"
		}
		t.Run(name, func(t *testing.T) {
			snapshot := countSnapshot("2026-11-05", 20000)
			snapshot.Breakdown = usage.Breakdown{
				InputTokens: new("9900"), CachedInputTokens: new("8000"), UncachedInputTokens: new("1900"),
				CacheWriteInputTokens: new("200"), OutputTokens: new("4100"),
				ReasoningOutputTokens: new("2300"), NonReasoningOutputTokens: new("1800"),
			}
			app := &counterWindow{snapshot: snapshot, cardView: card}
			view := app.renderDetails
			if card {
				view = app.render
			}
			preview := ui.NewTester(view, detailsWidth, detailHeight(snapshot))
			preview.SetScale(1.5)
			input, ok := preview.Find("9900")
			if !ok {
				t.Fatal("initial input count is missing")
			}

			snapshot.Breakdown = usage.Breakdown{
				InputTokens: new("10020"), CachedInputTokens: new("8080"), UncachedInputTokens: new("1940"),
				CacheWriteInputTokens: new("360"), OutputTokens: new("4180"),
				ReasoningOutputTokens: new("2340"), NonReasoningOutputTokens: new("1840"),
				CacheHitRatePercent: new("80.64"),
			}
			app.setSnapshot(snapshot, preview.Now(), true)
			preview.Frame()
			if !preview.FramePending() {
				t.Fatal("a detail-only update must request animation frames")
			}

			preview.Advance(countDuration / 2)
			preview.Frame()
			for _, value := range []string{"1,0005", "8070", "1935", "340", "4170", "2335", "1835", "2,0000", "缓存命中率 80.64%"} {
				if !preview.HasText(value) {
					t.Errorf("missing midpoint text %q", value)
				}
			}
			midpoint, ok := preview.Find("1,0005")
			if delta := midpoint.X + midpoint.W - input.X - input.W; !ok || delta < -0.5 || delta > 0.5 {
				t.Errorf("input count moved its right edge on carry: before %+v, during %+v", input, midpoint)
			}

			preview.Advance(countDuration / 2)
			preview.Frame()
			for _, value := range []string{"1,0020", "8080", "1940", "360", "4180", "2340", "1840"} {
				if !preview.HasText(value) {
					t.Errorf("missing final count %q", value)
				}
			}
			if preview.FramePending() {
				t.Error("settled counts should stop requesting frames")
			}

			snapshot.Breakdown.InputTokens = new("10200")
			app.setSnapshot(snapshot, preview.Now(), true)
			preview.Frame()
			preview.SetPreferences(ui.Preferences{ReduceMotion: true})
			if !preview.HasText("1,0200") || preview.FramePending() {
				t.Error("reduced motion must settle the count and stop requesting frames")
			}
		})
	}
}
