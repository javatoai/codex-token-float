package desktop

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/javatoai/codex-token-float/internal/usage"
)

func TestCountAnimationKeepsIntegerPrecision(t *testing.T) {
	now := time.Unix(100, 0)
	for _, duration := range []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 16 * time.Second} {
		for _, from := range []uint64{0, math.MaxUint64 - 8_000} {
			animation := newCountAnimation(from, math.MaxUint64, now, duration)
			last := from
			for elapsed := time.Duration(0); elapsed < duration; elapsed += time.Millisecond {
				value, moving := animation.valueAt(now.Add(elapsed))
				if !moving || value < last || value >= animation.to || (elapsed == 0 && value != from) {
					t.Fatalf("%v: %d -> %d at %v: previous %d, got %d (moving %v)", duration, from, animation.to, elapsed, last, value, moving)
				}
				last = value
			}
			if value, moving := animation.valueAt(now.Add(duration)); value != animation.to || moving {
				t.Fatalf("end of %v count: got %d (moving %v), want %d", duration, value, moving, animation.to)
			}
		}
		animation := newCountAnimation(math.MaxUint64-8_000, math.MaxUint64, now, duration)
		if value, _ := animation.valueAt(now.Add(duration / 2)); value != math.MaxUint64-4_000 {
			t.Fatalf("linear count halfway through %v: got %d, want %d", duration, value, uint64(math.MaxUint64-4_000))
		}
	}
}

func TestSnapshotAnimationKeepsProgress(t *testing.T) {
	now := time.Unix(100, 0)
	const day = "2026-11-05"
	app := &counterWindow{settings: defaultSettings()}
	app.setSnapshot(countSnapshot(day, 1000), now, true)
	app.setSnapshot(countSnapshot(day, 9000), now, true)
	if value, moving := app.animation.valueAt(now); value != 1000 || !moving {
		t.Fatalf("count did not start at the previous total: %d (moving %v)", value, moving)
	}
	before := app.animation
	retargetAt := now.Add(150 * time.Millisecond)
	current, _ := before.valueAt(retargetAt)
	unchanged := countSnapshot(day, 9000)
	unchanged.UpdatedAt = "10:00:01"
	app.setSnapshot(unchanged, retargetAt, true)
	if app.animation != before || app.snapshot.UpdatedAt != unchanged.UpdatedAt {
		t.Fatal("same total should refresh the exact snapshot without restarting the count")
	}
	app.setSnapshot(countSnapshot(day, 17_000), retargetAt, true)
	if value, moving := app.animation.valueAt(retargetAt); value != current || !moving || app.value() != "1,7000" {
		t.Fatalf("retarget: got %d (moving %v), exact value %q, want from %d and exact 1,7000", value, moving, app.value(), current)
	}
	if value, moving := app.animation.valueAt(retargetAt.Add(app.animation.duration)); value != 17_000 || moving {
		t.Fatalf("retarget end: got %d (moving %v), want 17000", value, moving)
	}
}

func TestSnapshotPacingBridgesEmptyRefreshes(t *testing.T) {
	now := time.Unix(100, 0)
	const day = "2026-11-05"
	snapshot := countSnapshot(day, 20_000)
	app := &counterWindow{snapshot: countSnapshot(day, 10_000), settings: defaultSettings()}
	app.setSnapshot(snapshot, now, true)
	first, pace := app.animation, app.countPace
	if first.duration != 6*time.Second {
		t.Fatalf("first growth duration = %v, want 6s", first.duration)
	}
	for _, gap := range []time.Duration{2 * time.Second, 4 * time.Second} {
		at := now.Add(gap)
		app.setSnapshot(snapshot, at, true)
		value, moving := app.animation.valueAt(at)
		if app.animation != first || app.countPace != pace || !moving || value <= 10_000 || value >= 20_000 {
			t.Fatalf("empty poll at %v interrupted growth: %d (moving %v), pace %+v", gap, value, moving, app.countPace)
		}
	}
	at := now.Add(6 * time.Second)
	app.setSnapshot(countSnapshot(day, 30_000), at, true)
	if app.animation.duration != 9*time.Second {
		t.Fatalf("six seconds between growth updates should roll for nine seconds, got %v", app.animation.duration)
	}
	at = at.Add(8 * time.Second)
	displayed, moving := app.animation.valueAt(at)
	if !moving || displayed >= 30_000 {
		t.Fatalf("should still be rolling after the eight-second gap: %d (moving %v)", displayed, moving)
	}
	app.setSnapshot(countSnapshot(day, 40_000), at, true)
	if app.animation.from != displayed || app.animation.duration != 9750*time.Millisecond {
		t.Fatalf("should resume at %d with a smoothed gap: %+v", displayed, app.animation)
	}
	end := at.Add(app.animation.duration)
	if value, moving := app.animation.valueAt(end); value != 40_000 || moving {
		t.Fatalf("count should settle at the latest target: %d (moving %v)", value, moving)
	}
	app.setSnapshot(countSnapshot("2026-11-06", 100), end, true)
	if app.countPace != (countPace{}) || app.animation.duration != 0 {
		t.Fatal("a new day should start with no growth history or animation")
	}
}

func TestCountPaceBounds(t *testing.T) {
	var pace countPace
	for _, choice := range []struct {
		interval, duration time.Duration
	}{
		{time.Second, 3 * time.Second}, {2 * time.Second, 6 * time.Second}, {5 * time.Second, 15 * time.Second},
	} {
		if got := pace.duration(choice.interval); got != choice.duration {
			t.Errorf("initial duration at %v = %v, want %v", choice.interval, got, choice.duration)
		}
	}
	now := time.Unix(100, 0)
	pace.recordGrowth(now)
	pace.recordGrowth(now.Add(time.Second))
	if got := pace.duration(2 * time.Second); got != 6*time.Second {
		t.Errorf("rapid updates should retain a three-poll window, got %v", got)
	}
	pace.recordGrowth(now.Add(2 * time.Minute))
	if pace.averageGap != 3250*time.Millisecond {
		t.Errorf("long idle gap should be capped before averaging, got %v", pace.averageGap)
	}
	pace = countPace{lastGrowth: now}
	pace.recordGrowth(now.Add(2 * time.Minute))
	if got := pace.duration(2 * time.Second); got != defaultCountLimit {
		t.Errorf("duration after a long pause = %v, want %v", got, defaultCountLimit)
	}
}

func TestFiveSecondRefreshCatchesUpAndKeepsFullRollingWindow(t *testing.T) {
	now := time.Unix(100, 0)
	const day = "2026-11-05"
	initial := countSnapshot(day, 1000)
	initial.Breakdown.InputTokens = new("600")
	first := countSnapshot(day, 10_000)
	first.Breakdown.InputTokens = new("6000")
	app := &counterWindow{snapshot: initial, settings: desktopSettings{RefreshSeconds: 5}}
	app.setSnapshot(first, now, true)
	if app.animation.duration != 15*time.Second || app.animation.catchUpDuration != 0 {
		t.Fatalf("initial increase should spread over fifteen seconds: %+v", app.animation)
	}

	check := func(at time.Time, wantTotal, wantInput uint64, wantMoving bool) {
		t.Helper()
		total, totalMoving := app.animation.valueAt(at)
		input, inputMoving := app.detailAnimations[detailInput].valueAt(at)
		if total != wantTotal || input != wantInput || totalMoving != wantMoving || inputMoving != wantMoving {
			t.Fatalf("%v: got total %d (%v), input %d (%v), want %d and %d (moving %v)", at, total, totalMoving, input, inputMoving, wantTotal, wantInput, wantMoving)
		}
	}
	app.setSnapshot(first, now.Add(5*time.Second), true)
	at := now.Add(10 * time.Second)
	check(at, 7000, 4200, true)
	second := countSnapshot(day, 13_000)
	second.Breakdown.InputTokens = new("8400")
	app.setSnapshot(second, at, true)
	check(at, 7000, 4200, true)
	check(at.Add(500*time.Millisecond), 8000, 4900, true)
	check(at.Add(time.Second), 9000, 5600, true)
	app.setSnapshot(second, at.Add(5*time.Second), true)
	check(at.Add(15*time.Second), 12_733, 8213, true)
	check(at.Add(16*time.Second), 13_000, 8400, false)
}

func TestChangingRefreshIntervalRetimesTotalAndDetails(t *testing.T) {
	now := time.Unix(100, 0)
	const day = "2026-11-05"
	initial := countSnapshot(day, 1_000)
	initial.Breakdown.InputTokens = new("800")
	next := countSnapshot(day, 3_000)
	next.Breakdown.InputTokens = new("2400")
	app := &counterWindow{snapshot: initial, settings: defaultSettings()}
	app.setSnapshot(next, now, true)

	check := func(at time.Time, wantTotal, wantInput uint64, wantActive bool) {
		t.Helper()
		total, totalActive := app.animation.valueAt(at)
		input, inputActive := app.detailAnimations[detailInput].valueAt(at)
		if total != wantTotal || input != wantInput || totalActive != wantActive || inputActive != wantActive {
			t.Fatalf("%v: got total %d (%v), input %d (%v), want %d and %d (moving %v)", at, total, totalActive, input, inputActive, wantTotal, wantInput, wantActive)
		}
	}
	at := now.Add(1500 * time.Millisecond)
	check(at, 1500, 1200, true)
	app.retimeAnimations(at, 5*time.Second)
	check(at, 1500, 1200, true)
	at = at.Add(4250 * time.Millisecond)
	check(at, 2250, 1800, true)
	app.retimeAnimations(at, time.Second)
	check(at, 2250, 1800, true)
	check(at.Add(1500*time.Millisecond), 2625, 2100, true)
	check(at.Add(3*time.Second), 3000, 2400, false)
	app.retimeAnimations(at.Add(3*time.Second), 2*time.Second)
	check(at.Add(3*time.Second), 3000, 2400, false)
}

func TestRetimingKeepsLatestTargetDeadline(t *testing.T) {
	now := time.Unix(100, 0)
	animation := newCountAnimation(100, 200, now, defaultCountLimit)
	at := now.Add(9 * time.Second)
	animation = animation.retime(at, defaultCountLimit)
	if value, moving := animation.valueAt(at.Add(500 * time.Millisecond)); value != 195 || !moving {
		t.Fatalf("retimed count should continue rising within the remaining second: %d (moving %v)", value, moving)
	}
	if value, moving := animation.valueAt(now.Add(defaultCountLimit)); value != 200 || moving {
		t.Fatalf("retimed count exceeded the original deadline: %d (moving %v)", value, moving)
	}
}

func TestSnapshotAnimationSnapsWhenNeeded(t *testing.T) {
	now := time.Unix(100, 0)
	const day = "2026-11-05"
	prior := countSnapshot(day, 100)
	missing := usage.Snapshot{Date: day, Status: usage.MissingDirectory}
	for _, tc := range []struct {
		name           string
		previous, next usage.Snapshot
		animate        bool
		want           string
	}{
		{"first snapshot", usage.Snapshot{}, prior, true, "100"},
		{"new day", prior, countSnapshot("2026-11-06", 200), true, "200"},
		{"decrease", prior, countSnapshot(day, 90), true, "90"},
		{"one token", prior, countSnapshot(day, 101), true, "101"},
		{"missing directory", prior, missing, true, "—"},
		{"directory restored", missing, prior, true, "100"},
		{"animation disabled", prior, countSnapshot(day, 200), false, "200"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &counterWindow{snapshot: tc.previous, settings: defaultSettings()}
			app.setSnapshot(tc.next, now, tc.animate)
			if !app.animation.started.IsZero() || app.value() != tc.want {
				t.Fatalf("got animation %+v and exact value %q, want immediate %q", app.animation, app.value(), tc.want)
			}
		})
	}
	app := &counterWindow{snapshot: prior, animation: newCountAnimation(1, 100, now, time.Second), settings: defaultSettings()}
	app.setSnapshot(countSnapshot(day, 90), now, true)
	if !app.animation.started.IsZero() {
		t.Fatal("a correction must cancel the active count even when the current frame is below the corrected total")
	}
}

func TestDetailAnimationRetargetsAndSnaps(t *testing.T) {
	now := time.Unix(100, 0)
	snapshot := countSnapshot("2026-11-05", 1000)
	snapshot.Breakdown.OutputTokens = new("100")
	app := &counterWindow{snapshot: snapshot, settings: defaultSettings()}

	snapshot.Breakdown.OutputTokens = new("200")
	app.setSnapshot(snapshot, now, true)
	retargetAt := now.Add(150 * time.Millisecond)
	middle, moving := app.detailAnimations[detailOutput].valueAt(retargetAt)
	if !moving || middle <= 100 || middle >= 200 {
		t.Fatalf("expected an intermediate output count, got %d (moving %v)", middle, moving)
	}
	snapshot.Breakdown.OutputTokens = new("300")
	app.setSnapshot(snapshot, retargetAt, true)
	if value, active := app.detailAnimations[detailOutput].valueAt(retargetAt); !active || value != middle {
		t.Fatalf("retargeted output started at %d (moving %v), want %d", value, active, middle)
	}

	// A one-token change to the target still has room to roll from the displayed count.
	snapshot.Breakdown.OutputTokens = new("301")
	app.setSnapshot(snapshot, retargetAt, true)
	if value, active := app.detailAnimations[detailOutput].valueAt(retargetAt); !active || value != middle {
		t.Fatalf("one-token retarget while rolling: got %d (moving %v), want %d", value, active, middle)
	}

	for _, value := range []*string{new("250"), nil, new("400"), new("401")} {
		snapshot.Breakdown.OutputTokens = value
		app.setSnapshot(snapshot, retargetAt, true)
		if _, active := app.detailAnimations[detailOutput].valueAt(retargetAt); active {
			t.Fatalf("expected an immediate display for output value %v", value)
		}
	}

	snapshot.Breakdown.OutputTokens = new("500")
	app.setSnapshot(snapshot, retargetAt, true)
	snapshot.Date = "2026-11-06"
	snapshot.Breakdown.OutputTokens = new("600")
	app.setSnapshot(snapshot, retargetAt, true)
	if _, active := app.detailAnimations[detailOutput].valueAt(retargetAt); active {
		t.Fatal("the first count of a new day must display immediately")
	}
}

func countSnapshot(date string, tokens uint64) usage.Snapshot {
	digits := strconv.FormatUint(tokens, 10)
	return usage.Snapshot{Date: date, Status: usage.OK, TotalTokens: digits, FormattedTotal: usage.FormatTokens(digits)}
}
