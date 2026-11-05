package desktop

import (
	"math/bits"
	"strconv"
	"time"

	"github.com/javatoai/codex-token-float/internal/usage"
)

const countDuration = 450 * time.Millisecond

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

func (app *counterWindow) setSnapshot(next usage.Snapshot, now time.Time, animate bool) {
	previous := app.snapshot
	app.snapshot = next
	if !animate || previous.Date != next.Date || !hasTotal(previous) || !hasTotal(next) {
		app.animation = countAnimation{}
		return
	}
	if previous.TotalTokens == next.TotalTokens {
		return
	}

	from, moving := app.animation.valueAt(now)
	app.animation = countAnimation{}
	old, oldErr := strconv.ParseUint(previous.TotalTokens, 10, 64)
	target, targetErr := strconv.ParseUint(next.TotalTokens, 10, 64)
	if oldErr != nil || targetErr != nil || target <= old {
		return
	}
	if !moving {
		from = old
	}
	if target-from > 1 {
		app.animation = countAnimation{from: from, to: target, started: now}
	}
}
