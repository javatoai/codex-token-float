package usage_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/javatoai/codex-token-float/internal/usage"
)

func TestCountsBeijingDay(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "2026", "11", "04", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeLog(t, path, cumulative("2026-11-04T15:59:59Z", 100)+
		cumulative("2026-11-04T16:00:00Z", 12300)+
		tokenEvent("2026-11-04T16:00:01Z", `"total_token_usage":{"total_tokens":12300},"last_token_usage":{"total_tokens":12200}`)+
		cumulative("2026-11-05T00:01:00+08:00", 14500))

	snapshot := usage.NewIndex(root).Scan(testTime(t, "2026-11-04T17:00:00Z"), false)
	if snapshot.Status != "ok" || snapshot.Date != "2026-11-05" || snapshot.TotalTokens != "14400" ||
		snapshot.FormattedTotal != "1,4400" || snapshot.FileCount != 1 || snapshot.EventCount != 2 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestUpdatesAChangingSession(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	now := testTime(t, "2026-11-05T00:30:00+08:00")
	index := usage.NewIndex(root)
	first := cumulative("2026-11-05T00:01:00+08:00", 10)
	second := strings.TrimSuffix(cumulative("2026-11-05T00:02:00+08:00", 30), "\n")
	writeLog(t, path, first+second[:len(second)-1])
	assertUsage(t, index.Scan(now, false), "10", 1, "ok")

	appendLog(t, path, second[len(second)-1:], now)
	assertUsage(t, index.Scan(now, false), "30", 2, "ok")
	appendLog(t, path, "\n"+cumulative("2026-11-05T00:03:00+08:00", 45), now)
	assertUsage(t, index.Scan(now, false), "45", 3, "ok")

	writeLog(t, path, cumulative("2026-11-05T00:04:00+08:00", 70))
	assertUsage(t, index.Scan(now, false), "70", 1, "ok")

	// Archiving and recreating a file can preserve its size and timestamp.
	if err := os.Rename(path, filepath.Join(root, "archived.log")); err != nil {
		t.Fatal(err)
	}
	writeLog(t, path, cumulative("2026-11-05T00:04:00+08:00", 80))
	assertUsage(t, index.Scan(now, false), "80", 1, "ok")

	writeLog(t, path, cumulative("2026-11-05T00:05:00+08:00", 15)+
		cumulative("2026-11-05T00:06:00+08:00", 40))
	assertUsage(t, index.Scan(now, false), "40", 2, "ok")
	assertUsage(t, index.Scan(now, true), "40", 2, "ok")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	assertUsage(t, index.Scan(now, false), "0", 0, "no_data")
}

func TestRecoversWhenSessionsAppearAndRollover(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions")
	path := filepath.Join(root, "session.jsonl")
	index := usage.NewIndex(root)
	now := testTime(t, "2026-11-05T23:59:59+08:00")
	assertUsage(t, index.Scan(now, false), "0", 0, "missing_directory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLog(t, path, cumulative("2026-11-05T23:59:58+08:00", 100))
	assertUsage(t, index.Scan(now, false), "100", 1, "ok")

	now = now.Add(2 * time.Second)
	assertUsage(t, index.Scan(now, false), "0", 0, "no_data")
	appendLog(t, path, cumulative("2026-11-06T00:00:00+08:00", 150), now)
	assertUsage(t, index.Scan(now, false), "50", 1, "ok")
}

func TestLegacyAndCounterRestart(t *testing.T) {
	root := t.TempDir()
	writeLog(t, filepath.Join(root, "legacy.jsonl"), strings.Repeat("a", 128*1024)+"\n"+
		tokenEvent("2026-11-05T00:01:00+08:00", `"last_token_usage":{"total_tokens":8,"cached_input_tokens":5}`))
	writeLog(t, filepath.Join(root, "current.jsonl"), cumulative("2026-11-05T00:01:00+08:00", 100)+
		cumulative("2026-11-05T00:02:00+08:00", 20))
	index := usage.NewIndex(root)
	assertUsage(t, index.Scan(testTime(t, "2026-11-05T00:30:00+08:00"), false), "128", 3, "ok")
}

func assertUsage(t *testing.T, snapshot usage.Snapshot, total string, events uint64, status usage.Status) {
	t.Helper()
	if snapshot.TotalTokens != total || snapshot.EventCount != events || snapshot.Status != status {
		t.Fatalf("expected %s tokens across %d events with status %s, got %+v", total, events, status, snapshot)
	}
}

func tokenEvent(timestamp, fields string) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{%s}}}`+"\n", timestamp, fields)
}

func cumulative(timestamp string, total uint64) string {
	return tokenEvent(timestamp, fmt.Sprintf(`"total_token_usage":{"total_tokens":%d}`, total))
}

func writeLog(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// Fixtures describe a historical day independently of the host clock.
	modified := testTime(t, "2026-11-05T00:30:00+08:00")
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func appendLog(t *testing.T, path, content string, modified time.Time) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("append log: %v, close log: %v", writeErr, closeErr)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
}

func testTime(t *testing.T, timestamp string) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
