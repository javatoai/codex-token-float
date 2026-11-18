package usage

import (
	"math"
	"testing"
)

func TestModelPricesReturnsACopy(t *testing.T) {
	prices := ModelPrices()
	first := prices[0]
	prices[0] = ModelPrice{}
	if got := ModelPrices()[0]; got != first {
		t.Fatalf("model catalog changed via returned slice: %+v", got)
	}
}

func TestModelPrices(t *testing.T) {
	usage := tokenUsage{
		Total: new(uint64(4_000_000)), Input: new(uint64(3_000_000)),
		CachedInput: new(uint64(2_000_000)), Output: new(uint64(1_000_000)),
	}
	for model, want := range map[string]string{
		"gpt-6.1-sol": "12.20", "gpt-6.2-sol": "12.20", "gpt-6.2-astra": "48.80",
		"gpt-6.1-terra": "6.10", "gpt-6-astra": "62.00", "gpt-6-sol": "12.40",
		"gpt-6-luna": "0.62", "gpt-5.6-sol": "24.80", "gpt-5.6-terra": "14.40", "gpt-5.6-luna": "1.44",
	} {
		var tally costTally
		tally.addUsage(usage, model)
		cost, _ := tally.estimate(false)
		if cost.USD == nil || *cost.USD != want || cost.Incomplete {
			t.Errorf("%s estimate = %+v, want $%s", model, cost, want)
		}
	}
}

func TestAPICostPrecision(t *testing.T) {
	var tally costTally
	usage := tokenUsage{Total: new(uint64(5_000)), Input: new(uint64(0)), CachedInput: new(uint64(0)), Output: new(uint64(5_000))}
	tally.addUsage(usage, "gpt-6-luna")
	tally.addUsage(usage, "gpt-6-luna")
	if cost, _ := tally.estimate(false); cost.USD == nil || *cost.USD != "0.01" {
		t.Fatalf("two $0.0025 responses must round to $0.01 after summing: %+v", cost)
	}

	var large costTally
	usage.Total, usage.Output = new(uint64(math.MaxUint64)), new(uint64(math.MaxUint64))
	large.addUsage(usage, "gpt-6-astra")
	if cost, _ := large.estimate(false); cost.USD == nil || *cost.USD != "922337203685477.58" {
		t.Fatalf("large estimate = %+v, want $922337203685477.58", cost)
	}
}

func TestMissingSubdivisionsDoNotInvalidateParentCosts(t *testing.T) {
	usage := tokenUsage{
		Total: new(uint64(1_100_000)), Input: new(uint64(1_000_000)),
		CachedInput: new(uint64(600_000)), Output: new(uint64(100_000)),
		CacheWriteInput: new(uint64(500_000)),
	}
	var tally costTally
	tally.addUsage(usage, "gpt-6.2-sol")
	total, details := tally.estimate(false)
	if total.USD == nil || *total.USD != "1.86" || total.Incomplete {
		t.Fatalf("unexpected total cost: %+v", total)
	}
	for name, amount := range map[string]APICost{
		"invalid cache write": details.CacheWriteInput, "missing reasoning": details.ReasoningOutput,
		"missing non-reasoning": details.NonReasoningOutput,
	} {
		if amount.USD != nil || !amount.Incomplete {
			t.Errorf("%s must be unavailable: %+v", name, amount)
		}
	}
	usage.ReasoningOutput = new(uint64(90_000))
	tally.addUsage(usage, "gpt-6.2-sol")
	_, details = tally.estimate(false)
	if output := details.Output; output.USD == nil || *output.USD != "2.00" || output.Incomplete {
		t.Fatalf("unexpected output cost: %+v", output)
	}
	if reasoning := details.ReasoningOutput; reasoning.USD == nil || *reasoning.USD != "0.90" || !reasoning.Incomplete {
		t.Fatalf("expected a partial reasoning subtotal: %+v", reasoning)
	}
}
