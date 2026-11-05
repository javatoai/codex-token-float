package usage

import (
	"fmt"
	"math"
	"math/bits"
	"slices"
	"strconv"
)

type tokenUsage struct {
	Total           *uint64 `json:"total_tokens"`
	Input           *uint64 `json:"input_tokens"`
	CachedInput     *uint64 `json:"cached_input_tokens"`
	CacheWriteInput *uint64 `json:"cache_write_input_tokens"`
	Output          *uint64 `json:"output_tokens"`
	ReasoningOutput *uint64 `json:"reasoning_output_tokens"`
}

func (u tokenUsage) difference(previous tokenUsage) tokenUsage {
	return tokenUsage{
		Total:           subtract(u.Total, previous.Total),
		Input:           subtract(u.Input, previous.Input),
		CachedInput:     subtract(u.CachedInput, previous.CachedInput),
		CacheWriteInput: subtract(u.CacheWriteInput, previous.CacheWriteInput),
		Output:          subtract(u.Output, previous.Output),
		ReasoningOutput: subtract(u.ReasoningOutput, previous.ReasoningOutput),
	}
}

func subtract(current, previous *uint64) *uint64 {
	if current == nil || previous == nil || *current < *previous {
		return nil
	}
	delta := *current - *previous
	return &delta
}

const (
	input = iota
	cachedInput
	cacheWriteInput
	output
	reasoningOutput
	detailCount
)

type tally struct {
	tokens     uint64
	events     uint64
	details    [detailCount]uint64
	missing    [detailCount]bool
	incomplete bool
}

func (t *tally) addUsage(usage tokenUsage) {
	if usage.Total == nil {
		t.incomplete = true
		return
	}
	if *usage.Total == 0 {
		return
	}
	t.tokens = saturatingAdd(t.tokens, *usage.Total)
	t.events = saturatingAdd(t.events, 1)

	fields := [detailCount]*uint64{
		input: usage.Input, cachedInput: usage.CachedInput, cacheWriteInput: usage.CacheWriteInput,
		output: usage.Output, reasoningOutput: usage.ReasoningOutput,
	}
	if usage.Input != nil && usage.CachedInput != nil && *usage.CachedInput > *usage.Input {
		fields[cachedInput] = nil
	}
	if usage.Output != nil && usage.ReasoningOutput != nil && *usage.ReasoningOutput > *usage.Output {
		fields[reasoningOutput] = nil
	}
	for field, value := range fields {
		if value == nil {
			t.missing[field] = true
		} else {
			t.details[field] = saturatingAdd(t.details[field], *value)
		}
	}
}

func (t *tally) add(other tally) {
	t.tokens = saturatingAdd(t.tokens, other.tokens)
	t.events = saturatingAdd(t.events, other.events)
	t.incomplete = t.incomplete || other.incomplete
	for field, value := range other.details {
		t.details[field] = saturatingAdd(t.details[field], value)
		t.missing[field] = t.missing[field] || other.missing[field]
	}
}

type Breakdown struct {
	InputTokens              *string `json:"inputTokens"`
	CachedInputTokens        *string `json:"cachedInputTokens"`
	UncachedInputTokens      *string `json:"uncachedInputTokens"`
	CacheWriteInputTokens    *string `json:"cacheWriteInputTokens"`
	OutputTokens             *string `json:"outputTokens"`
	ReasoningOutputTokens    *string `json:"reasoningOutputTokens"`
	NonReasoningOutputTokens *string `json:"-"`
	CacheHitRatePercent      *string `json:"cacheHitRatePercent"`
}

func (t tally) breakdown() Breakdown {
	if t.incomplete {
		return Breakdown{}
	}
	result := Breakdown{
		InputTokens:           t.detail(input),
		CachedInputTokens:     t.detail(cachedInput),
		CacheWriteInputTokens: t.detail(cacheWriteInput),
		OutputTokens:          t.detail(output),
		ReasoningOutputTokens: t.detail(reasoningOutput),
	}
	if result.OutputTokens != nil && result.ReasoningOutputTokens != nil {
		value := strconv.FormatUint(t.details[output]-t.details[reasoningOutput], 10)
		result.NonReasoningOutputTokens = &value
	}
	if result.InputTokens != nil && result.CachedInputTokens != nil {
		total, cached := t.details[input], t.details[cachedInput]
		uncached := strconv.FormatUint(total-cached, 10)
		result.UncachedInputTokens = &uncached
		if total > 0 {
			rate := cacheHitPercent(cached, total)
			result.CacheHitRatePercent = &rate
		}
	}
	return result
}

func (t tally) detail(field int) *string {
	if t.missing[field] {
		return nil
	}
	value := strconv.FormatUint(t.details[field], 10)
	return &value
}

func (t tally) missingDetails() bool {
	return slices.Contains(t.missing[:], true)
}

func cacheHitPercent(cached, total uint64) string {
	// Hundredths of a percent, rounded half up without a float or overflow.
	high, low := bits.Mul64(cached, 10_000)
	hundredths, remainder := bits.Div64(high, low, total)
	if remainder >= total-remainder {
		hundredths++
	}
	return fmt.Sprintf("%d.%02d", hundredths/100, hundredths%100)
}

func saturatingAdd(a, b uint64) uint64 {
	if math.MaxUint64-a < b {
		return math.MaxUint64
	}
	return a + b
}
