package desktop

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestModelPriceTableAtHighDPI(t *testing.T) {
	want := []string{
		"美元／百万 Token", "模型", "未缓存输入", "缓存命中", "输出",
		"GPT-6.1 Sol", "2.00", "0.10", "10.00",
		"GPT-6.2 Sol", "2.00", "0.10", "10.00",
		"GPT-6.2 Astra", "8.00", "0.40", "40.00",
		"GPT-6.1 Terra", "1.00", "0.05", "5.00",
		"GPT-6 Astra", "10.00", "1.00", "50.00",
		"GPT-6 Sol", "2.00", "0.20", "10.00",
		"GPT-6 Luna", "0.10", "0.01", "0.50",
		"GPT-5.6 Sol（优惠价）", "4.00", "0.40", "20.00",
		"GPT-5.6 Terra", "2.00", "0.20", "12.00",
		"GPT-5.6 Luna", "0.20", "0.02", "1.20",
	}
	preview := ui.NewTester(renderModelPrices, modelPriceWidth, modelPriceHeight)
	preview.SetScale(1.5)
	for _, dark := range []bool{false, true} {
		preview.SetDark(dark)
		if got := preview.Texts(); !slices.Equal(got, want) {
			t.Fatalf("dark=%v: unexpected model price table: %v", dark, got)
		}
		for _, label := range want {
			rect, ok := preview.Find(label)
			if !ok || rect.X < 0 || rect.Y < 0 || rect.X+rect.W > modelPriceWidth || rect.Y+rect.H > modelPriceHeight {
				t.Errorf("dark=%v: %q outside the table window: %+v", dark, label, rect)
			}
		}
	}
}
