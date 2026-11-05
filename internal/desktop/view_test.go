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
