package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/javatoai/codex-token-float/internal/usage"
)

func TestMonitorAppliesRefreshIntervalAndManualRebuild(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		path := filepath.Join(root, "usage.jsonl")
		stamp := time.Now().UTC().Format(time.RFC3339)
		modified := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
		writeUsage := func(tokens int) {
			line := fmt.Sprintf(`{"timestamp":%q,"type":"token_usage_record","payload":{"response_id":"response","usage":{"total_tokens":%d}}}`+"\n", stamp, tokens)
			if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, modified, modified); err != nil {
				t.Fatal(err)
			}
		}
		writeUsage(10)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		intervals := make(chan time.Duration, 1)
		refresh := make(chan struct{}, 1)
		var snapshots []usage.Snapshot
		go monitor(ctx, root, defaultSettings().interval(), intervals, refresh, func(snapshot usage.Snapshot) {
			snapshots = append(snapshots, snapshot)
		})
		check := func(scans int, tokens string) {
			t.Helper()
			synctest.Wait()
			if len(snapshots) != scans || snapshots[scans-1].TotalTokens != tokens {
				t.Fatalf("wanted %d scans ending at %s tokens, got %+v", scans, tokens, snapshots)
			}
		}
		check(1, "10")
		time.Sleep(time.Second)
		check(1, "10")
		time.Sleep(time.Second)
		check(2, "10")

		intervals <- 5 * time.Second
		check(2, "10")
		time.Sleep(4 * time.Second)
		check(2, "10")
		// A rewrite with the same size and mtime needs a manual rebuild.
		writeUsage(20)
		refresh <- struct{}{}
		check(3, "20")
		time.Sleep(time.Second)
		check(4, "20")

		intervals <- time.Second
		check(4, "20")
		time.Sleep(time.Second)
		check(5, "20")
	})
}
