package desktop

import (
	"math/bits"
	"strconv"
	"time"

	"github.com/javatoai/codex-token-float/internal/usage"
)

const countDuration = 450 * time.Millisecond

const (
	detailInput = iota
	detailCached
	detailUncached
	detailCacheWrite
	detailOutput
	detailReasoning
	detailNonReasoning
	detailCount
)

func detailValues(b usage.Breakdown) [detailCount]*string {
	return [detailCount]*string{
		detailInput:        b.InputTokens,
		detailCached:       b.CachedInputTokens,
		detailUncached:     b.UncachedInputTokens,
		detailCacheWrite:   b.CacheWriteInputTokens,
		detailOutput:       b.OutputTokens,
		detailReasoning:    b.ReasoningOutputTokens,
		detailNonReasoning: b.NonReasoningOutputTokens,
	}
}

type countAnimation struct {
	from, to uint64
	started  time.Time
}

func (a countAnimation) valueAt(now time.Time) (value uint64, moving bool) {
	elapsed := now.Sub(a.started)
	if a.started.IsZero() || elapsed >= countDuration {
		return a.to, false
	}

	// Cubic ease-out, using milliseconds and a wide product to keep the
	// interpolation exact even when the full uint64 range is in use.
	const steps = uint64(countDuration / time.Millisecond)
	const denominator = steps * steps * steps
	left := steps - uint64(max(elapsed, 0)/time.Millisecond)
	numerator := denominator - left*left*left
	high, low := bits.Mul64(a.to-a.from, numerator)
	increase, _ := bits.Div64(high, low, denominator)
	return a.from + increase, true
}

func hasTotal(snapshot usage.Snapshot) bool {
	return snapshot.Status != "" && snapshot.Status != usage.MissingDirectory
}

func (a countAnimation) retarget(previous, next *string, now time.Time) countAnimation {
	if previous == nil || next == nil {
		return countAnimation{}
	}
	if *previous == *next {
		return a
	}
	old, oldErr := strconv.ParseUint(*previous, 10, 64)
	target, targetErr := strconv.ParseUint(*next, 10, 64)
	if oldErr != nil || targetErr != nil || target <= old || target-old == 1 {
		return countAnimation{}
	}
	from, moving := a.valueAt(now)
	if !moving {
		from = old
	}
	return countAnimation{from: from, to: target, started: now}
}

func (app *counterWindow) resetAnimations() {
	app.animation = countAnimation{}
	app.detailAnimations = [detailCount]countAnimation{}
}

func (app *counterWindow) setSnapshot(next usage.Snapshot, now time.Time, animate bool) {
	previous := app.snapshot
	app.snapshot = next
	if !animate || previous.Date != next.Date || !hasTotal(previous) || !hasTotal(next) {
		app.resetAnimations()
		return
	}

	app.animation = app.animation.retarget(&previous.TotalTokens, &next.TotalTokens, now)
	oldDetails := detailValues(previous.Breakdown)
	for i, value := range detailValues(next.Breakdown) {
		app.detailAnimations[i] = app.detailAnimations[i].retarget(oldDetails[i], value, now)
	}
}
