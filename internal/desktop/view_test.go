package desktop

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/javatoai/codex-token-float/internal/usage"
)

func TestCompactBackgroundMatchesCard(t *testing.T) {
	app := &counterWindow{snapshot: countSnapshot("2026-11-05", 1234), settings: defaultSettings()}
	compact := ui.NewTester(app.render, windowWidth, windowHeight)
	width, height := app.detailSize()
	card := ui.NewTester(app.renderDetails, width, height)
	for _, dark := range []bool{false, true} {
		compact.SetDark(dark)
		card.SetDark(dark)
		got, want := compact.Image().RGBAAt(0, 0), card.Image().RGBAAt(0, 0)
		if got != want || got.A != 255 {
			t.Errorf("dark=%v: compact background = %v, want opaque card background %v", dark, got, want)
		}
	}
}

func TestDetailsFitAtHighDPI(t *testing.T) {
	message := "部分用量缺少分类明细"
	app := &counterWindow{settings: defaultSettings(), snapshot: usage.Snapshot{
		Status:         usage.PartialError,
		Date:           "2026-11-05",
		UpdatedAt:      "23:59:59",
		FormattedTotal: usage.FormatTokens("18446744073709551615"),
		Message:        &message,
		APICost:        usage.APICost{USD: new("922337203685477.58"), Incomplete: true},
		APICostBreakdown: usage.APICostBreakdown{
			Input:              usage.APICost{USD: new("461168601842738.79"), Incomplete: true},
			CachedInput:        usage.APICost{USD: new("100.24")},
			UncachedInput:      usage.APICost{USD: new("461168601842638.55")},
			CacheWriteInput:    usage.APICost{USD: new("0.02")},
			Output:             usage.APICost{USD: new("2.00")},
			ReasoningOutput:    usage.APICost{USD: new("0.60")},
			NonReasoningOutput: usage.APICost{USD: new("1.40")},
		},
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
	width, height := app.detailSize()
	preview := ui.NewTester(app.renderDetails, width, height)
	preview.SetScale(1.5)
	preview.SetDark(true)
	if got := preview.Image().Bounds().Dx(); got < width*3/2 || got > (width*3+1)/2 {
		t.Fatalf("render width at 150%% scaling = %d", got)
	}
	for _, label := range []string{
		app.snapshot.FormattedTotal, "缓存命中率 96.45%", "$922337203685477.58（部分）",
		"输入总量", "$461168601842738.79（部分）", "命中缓存", "$100.24",
		"未命中输入", "$461168601842638.55", "其中写入缓存", "$0.02", "输出总量", "$2.00",
		"其中推理", "$0.60", "非推理输出", "$1.40", "7000",
		message,
	} {
		rect, ok := preview.Find(label)
		if !ok || rect.X < 0 || rect.Y < 0 || rect.X+rect.W > float32(width) || rect.Y+rect.H > float32(height) {
			t.Errorf("%q not visible within the details window: %+v", label, rect)
		}
	}
	if preview.HasText("北京时间") || preview.HasText(app.snapshot.UpdatedAt) {
		t.Fatalf("details must omit the date and refresh time: %v", preview.Texts())
	}
	tokens, hasTokens := preview.Find(app.snapshot.FormattedTotal)
	cost, hasCost := preview.Find("$922337203685477.58（部分）")
	if !hasTokens || !hasCost || cost.X < tokens.X+tokens.W || cost.Y < tokens.Y || cost.Y+cost.H > tokens.Y+tokens.H {
		t.Fatalf("total cost must sit to the right on the Token total row: tokens %+v, cost %+v", tokens, cost)
	}

	app.snapshot.Breakdown.CacheWriteInputTokens = new("0")
	app.snapshot.Breakdown.CacheHitRatePercent = nil
	preview.SetSize(app.detailSize())
	if preview.HasText("其中写入缓存") || preview.HasText("$0.02") || !preview.HasText("缓存命中率 —") {
		t.Fatalf("unexpected rows with zero cache writes and unknown rate: %v", preview.Texts())
	}
	app.snapshot.APICost = usage.APICost{Incomplete: true}
	preview.Frame()
	if !preview.HasText("$—") || preview.HasText("$922337203685477.58（部分）") {
		t.Fatalf("unavailable cost must replace the prior subtotal: %v", preview.Texts())
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
			snapshot.APICost = usage.APICost{USD: new("0.00")}
			snapshot.APICostBreakdown.Input = usage.APICost{USD: new("0.99")}
			snapshot.Breakdown = usage.Breakdown{
				InputTokens: new("9900"), CachedInputTokens: new("8000"), UncachedInputTokens: new("1900"),
				CacheWriteInputTokens: new("200"), OutputTokens: new("4100"),
				ReasoningOutputTokens: new("2300"), NonReasoningOutputTokens: new("1800"),
			}
			app := &counterWindow{snapshot: snapshot, cardView: card, settings: defaultSettings()}
			view := app.renderDetails
			if card {
				view = app.render
			}
			width, height := app.detailSize()
			preview := ui.NewTester(view, width, height)
			preview.SetScale(1.5)
			if preview.HasText("今日 API 估算") || !preview.HasText("$0.00") {
				t.Fatalf("expected the amount without a visible label: %v", preview.Texts())
			}
			input, ok := preview.Find("9900")
			if !ok {
				t.Fatal("initial input count is missing")
			}
			label, hasLabel := preview.Find("输入总量")
			amount, hasAmount := preview.Find("$0.99")
			if !hasLabel || !hasAmount || amount.X < label.X+label.W || amount.X+amount.W > input.X || amount.Y != input.Y {
				t.Fatalf("cost must sit between the label and count: label %+v, cost %+v, count %+v", label, amount, input)
			}

			snapshot.Breakdown = usage.Breakdown{
				InputTokens: new("10020"), CachedInputTokens: new("8080"), UncachedInputTokens: new("1940"),
				CacheWriteInputTokens: new("360"), OutputTokens: new("4180"),
				ReasoningOutputTokens: new("2340"), NonReasoningOutputTokens: new("1840"),
				CacheHitRatePercent: new("80.64"),
			}
			snapshot.APICost = usage.APICost{USD: new("1.86")}
			snapshot.APICostBreakdown.Input = usage.APICost{USD: new("1.25")}
			app.setSnapshot(snapshot, preview.Now(), true)
			preview.Frame()
			if !preview.HasText("$1.86") || !preview.HasText("$1.25") {
				t.Fatalf("cost should update with the snapshot: %v", preview.Texts())
			}
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

			app.settings.ShowUSD = false
			preview.SetSize(app.detailSize())
			for _, label := range preview.Texts() {
				if strings.HasPrefix(label, "$") {
					t.Errorf("hidden cost is still visible: %q", label)
				}
			}
			if !preview.HasText("输入总量") || !preview.HasText("1,0200") || !preview.HasText("输出总量") {
				t.Fatalf("Token breakdown is missing in the two-column view: %v", preview.Texts())
			}
			app.settings.ShowUSD = true
			preview.SetSize(app.detailSize())
			if !preview.HasText("$1.86") || !preview.HasText("$1.25") {
				t.Fatalf("the current amounts should reappear when enabled: %v", preview.Texts())
			}
		})
	}
}
