package desktop

import (
	"math/bits"
	"strconv"
	"time"

	"github.com/javatoai/codex-token-float/internal/usage"
)

const defaultCountLimit = 10 * time.Second

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
	from, to        uint64
	started         time.Time
	duration        time.Duration
	deadline        time.Time
	catchUpTo       uint64
	catchUpDuration time.Duration
}

type countPace struct {
	lastGrowth time.Time
	averageGap time.Duration
}

func (p *countPace) recordGrowth(now time.Time) {
	if !p.lastGrowth.IsZero() {
		// Limit the effect of long idle periods on the next animation.
		gap := min(now.Sub(p.lastGrowth), defaultCountLimit)
		if gap > 0 {
			if p.averageGap == 0 {
				p.averageGap = gap
			} else {
				p.averageGap += (gap - p.averageGap) / 4
			}
		}
	}
	p.lastGrowth = now
}

func (p countPace) duration(refreshInterval time.Duration) time.Duration {
	window := 3 * refreshInterval
	limit := max(defaultCountLimit, window)
	return min(limit, max(window, p.averageGap+p.averageGap/2))
}

func (a countAnimation) valueAt(now time.Time) (value uint64, moving bool) {
	elapsed := max(now.Sub(a.started), 0)
	if a.duration <= 0 || elapsed >= a.duration {
		return a.to, false
	}
	if a.catchUpDuration == 0 {
		return interpolateCount(a.from, a.to, elapsed, a.duration), true
	}
	if elapsed < a.catchUpDuration {
		return interpolateCount(a.from, a.catchUpTo, elapsed, a.catchUpDuration), true
	}
	return interpolateCount(a.catchUpTo, a.to, elapsed-a.catchUpDuration, a.duration-a.catchUpDuration), true
}

func interpolateCount(from, to uint64, elapsed, duration time.Duration) uint64 {
	// Use a wide product to interpolate exactly over the full uint64 range.
	high, low := bits.Mul64(to-from, uint64(elapsed))
	increase, _ := bits.Div64(high, low, uint64(duration))
	return from + increase
}

func newCountAnimation(from, to uint64, now time.Time, duration time.Duration) countAnimation {
	if to <= from || to-from == 1 || duration <= 0 {
		return countAnimation{}
	}
	return countAnimation{
		from: from, to: to, started: now,
		duration: duration, deadline: now.Add(max(defaultCountLimit, duration)),
	}
}

func hasTotal(snapshot usage.Snapshot) bool {
	return snapshot.Status != "" && snapshot.Status != usage.MissingDirectory
}

func (a countAnimation) retarget(previous, next *string, now time.Time, duration time.Duration) countAnimation {
	if previous == nil || next == nil {
		return countAnimation{}
	}
	if *previous == *next {
		return a
	}
	old, oldErr := strconv.ParseUint(*previous, 10, 64)
	target, targetErr := strconv.ParseUint(*next, 10, 64)
	if oldErr != nil || targetErr != nil || target <= old {
		return countAnimation{}
	}
	from, moving := a.valueAt(now)
	if !moving {
		return newCountAnimation(old, target, now, duration)
	}

	nextAnimation := newCountAnimation(from, target, now, duration)
	catchUpDuration := min(time.Second, nextAnimation.duration/10)
	if catchUpDuration > 0 && from < old {
		// Keep most of the outstanding tokens for the slower part of the animation.
		catchUpTo := from + min(old-from, (target-from)/3)
		if catchUpTo > interpolateCount(from, target, catchUpDuration, nextAnimation.duration) {
			nextAnimation.catchUpTo = catchUpTo
			nextAnimation.catchUpDuration = catchUpDuration
			if duration > defaultCountLimit {
				// Leave the full fifteen seconds after catching up in five-second mode.
				nextAnimation.duration += catchUpDuration
				nextAnimation.deadline = now.Add(nextAnimation.duration)
			}
		}
	}
	return nextAnimation
}

func (a countAnimation) retime(now time.Time, duration time.Duration) countAnimation {
	if from, moving := a.valueAt(now); moving {
		if a.to-from == 1 {
			return countAnimation{}
		}
		a.from, a.started = from, now
		a.duration = min(duration, a.deadline.Sub(now))
		a.catchUpTo, a.catchUpDuration = 0, 0
	}
	return a
}

func (app *counterWindow) resetAnimations() {
	app.animation = countAnimation{}
	app.detailAnimations = [detailCount]countAnimation{}
	app.countPace = countPace{}
}

func (app *counterWindow) retimeAnimations(now time.Time, refreshInterval time.Duration) {
	duration := app.countPace.duration(refreshInterval)
	app.animation = app.animation.retime(now, duration)
	for i, animation := range app.detailAnimations {
		app.detailAnimations[i] = animation.retime(now, duration)
	}
}

func (app *counterWindow) setSnapshot(next usage.Snapshot, now time.Time, animate bool) {
	previous := app.snapshot
	app.snapshot = next
	if !animate || previous.Date != next.Date || !hasTotal(previous) || !hasTotal(next) {
		app.resetAnimations()
		return
	}

	oldTotal, oldErr := strconv.ParseUint(previous.TotalTokens, 10, 64)
	newTotal, newErr := strconv.ParseUint(next.TotalTokens, 10, 64)
	if oldErr != nil || newErr != nil || newTotal < oldTotal {
		app.resetAnimations()
		return
	}
	if newTotal > oldTotal {
		app.countPace.recordGrowth(now)
	}
	duration := app.countPace.duration(app.settings.interval())
	app.animation = app.animation.retarget(&previous.TotalTokens, &next.TotalTokens, now, duration)
	oldDetails := detailValues(previous.Breakdown)
	for i, value := range detailValues(next.Breakdown) {
		app.detailAnimations[i] = app.detailAnimations[i].retarget(oldDetails[i], value, now, duration)
	}
}
