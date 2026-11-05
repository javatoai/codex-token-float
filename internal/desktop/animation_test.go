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
	for _, from := range []uint64{0, math.MaxUint64 - 8_000} {
		animation := countAnimation{from: from, to: math.MaxUint64, started: now}
		last := from
		for elapsed := time.Duration(0); elapsed < countDuration; elapsed += time.Millisecond {
			value, moving := animation.valueAt(now.Add(elapsed))
			if !moving || value < last || value >= animation.to || (elapsed == 0 && value != from) {
				t.Fatalf("%d -> %d at %v: previous %d, got %d (moving %v)", from, animation.to, elapsed, last, value, moving)
			}
			last = value
		}
		if value, moving := animation.valueAt(now.Add(countDuration)); value != animation.to || moving {
			t.Fatalf("end of count: got %d (moving %v), want %d", value, moving, animation.to)
		}
	}
	animation := countAnimation{from: math.MaxUint64 - 8_000, to: math.MaxUint64, started: now}
	if value, _ := animation.valueAt(now.Add(countDuration / 2)); value != math.MaxUint64-1_000 {
		t.Fatalf("cubic ease-out at halfway: got %d, want %d", value, uint64(math.MaxUint64-1_000))
	}
}

func TestSnapshotAnimationKeepsProgress(t *testing.T) {
	now := time.Unix(100, 0)
	const day = "2026-11-05"
	app := &counterWindow{}
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
	if value, moving := app.animation.valueAt(retargetAt.Add(countDuration)); value != 17_000 || moving {
		t.Fatalf("retarget end: got %d (moving %v), want 17000", value, moving)
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
			app := &counterWindow{snapshot: tc.previous}
			app.setSnapshot(tc.next, now, tc.animate)
			if !app.animation.started.IsZero() || app.value() != tc.want {
				t.Fatalf("got animation %+v and exact value %q, want immediate %q", app.animation, app.value(), tc.want)
			}
		})
	}
	app := &counterWindow{snapshot: prior, animation: countAnimation{from: 1, to: 100, started: now}}
	app.setSnapshot(countSnapshot(day, 90), now, true)
	if !app.animation.started.IsZero() {
		t.Fatal("a correction must cancel the active count even when the current frame is below the corrected total")
	}
}

func countSnapshot(date string, tokens uint64) usage.Snapshot {
	digits := strconv.FormatUint(tokens, 10)
	return usage.Snapshot{Date: date, Status: usage.OK, TotalTokens: digits, FormattedTotal: usage.FormatTokens(digits)}
}
