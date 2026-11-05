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

func TestCountsOriginalResponsesAcrossActiveAndArchivedLogs(t *testing.T) {
	home := t.TempDir()
	active := filepath.Join(home, "sessions")
	archive := filepath.Join(home, "archived_sessions")
	for _, root := range []string{active, archive} {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	before := "2026-11-04T15:59:59Z"
	today := "2026-11-04T16:00:01Z"
	writeLog(t, filepath.Join(archive, "parent.jsonl"), metadata("parent")+
		response(before, "parent", "older", 100)+response(today, "parent", "shared", 25))
	oldModifiedTime := testTime(t, before)
	if err := os.Chtimes(filepath.Join(archive, "parent.jsonl"), oldModifiedTime, oldModifiedTime); err != nil {
		t.Fatal(err)
	}
	writeLog(t, filepath.Join(active, "parent-copy.jsonl"), metadata("parent")+
		response(today, "parent", "older", 100)+response(today, "parent", "shared", 25))
	writeLog(t, filepath.Join(active, "child.jsonl"), metadata("child")+
		cumulative(today, 1_000_000)+response(today, "parent", "inherited", 500)+
		response(today, "child", "new", 17))

	index := usage.NewIndex(active)
	snapshot := index.Scan(testTime(t, "2026-11-04T17:00:00Z"), false)
	if snapshot.TotalTokens != "42" || snapshot.EventCount != 2 || snapshot.Date != "2026-11-05" {
		t.Fatalf("expected two distinct responses totaling 42, got %+v", snapshot)
	}
}

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
	if snapshot.Status != usage.PartialError || snapshot.Date != "2026-11-05" || snapshot.TotalTokens != "14400" ||
		snapshot.FormattedTotal != "1,4400" || snapshot.FileCount != 1 || snapshot.EventCount != 2 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestUpdatesAChangingSession(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	now := testTime(t, "2026-11-05T00:30:00+08:00")
	index := usage.NewIndex(root)
	first := response("2026-11-05T00:01:00+08:00", "thread", "r1", 10)
	second := strings.TrimSuffix(response("2026-11-05T00:02:00+08:00", "thread", "r2", 20), "\n")
	writeLog(t, path, first+second[:len(second)-1])
	assertUsage(t, index.Scan(now, false), "10", 1, "ok")

	appendLog(t, path, second[len(second)-1:], now)
	assertUsage(t, index.Scan(now, false), "30", 2, "ok")
	appendLog(t, path, "\n"+response("2026-11-05T00:03:00+08:00", "thread", "r3", 15), now)
	assertUsage(t, index.Scan(now, false), "45", 3, "ok")

	writeLog(t, path, response("2026-11-05T00:04:00+08:00", "thread", "r4", 70))
	assertUsage(t, index.Scan(now, false), "70", 1, "ok")

	// Archiving and recreating a file can preserve its size and timestamp.
	if err := os.Rename(path, filepath.Join(root, "archived.log")); err != nil {
		t.Fatal(err)
	}
	writeLog(t, path, response("2026-11-05T00:04:00+08:00", "thread", "r4", 80))
	assertUsage(t, index.Scan(now, false), "80", 1, "ok")

	writeLog(t, path, response("2026-11-05T00:05:00+08:00", "thread", "r5", 15)+
		response("2026-11-05T00:06:00+08:00", "thread", "r6", 25))
	assertUsage(t, index.Scan(now, false), "40", 2, "ok")
	assertUsage(t, index.Scan(now, true), "40", 2, "ok")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	assertUsage(t, index.Scan(now, false), "0", 0, "no_data")
}

func TestUpdatesWhileSessionLogIsOpen(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	now := testTime(t, "2026-11-05T00:30:00+08:00")
	index := usage.NewIndex(root)
	if _, err := writer.WriteString(response("2026-11-05T00:01:00+08:00", "thread", "r1", 10)); err != nil {
		t.Fatal(err)
	}
	assertUsage(t, index.Scan(now, false), "10", 1, "ok")
	if _, err := writer.WriteString(response("2026-11-05T00:02:00+08:00", "thread", "r2", 20)); err != nil {
		t.Fatal(err)
	}
	assertUsage(t, index.Scan(now.Add(2*time.Second), false), "30", 2, "ok")
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
	writeLog(t, path, response("2026-11-05T23:59:58+08:00", "thread", "r1", 100))
	assertUsage(t, index.Scan(now, false), "100", 1, "ok")

	now = now.Add(2 * time.Second)
	assertUsage(t, index.Scan(now, false), "0", 0, "no_data")
	appendLog(t, path, response("2026-11-06T00:00:00+08:00", "thread", "r2", 50), now)
	assertUsage(t, index.Scan(now, false), "50", 1, "ok")
}

func TestLegacyUsesOnlyConfirmedIncrements(t *testing.T) {
	root := t.TempDir()
	writeLog(t, filepath.Join(root, "legacy.jsonl"), strings.Repeat("a", 128*1024)+"\n"+
		tokenEvent("2026-11-05T00:01:00+08:00", `"last_token_usage":{"total_tokens":8,"cached_input_tokens":5}`))
	writeLog(t, filepath.Join(root, "current.jsonl"), cumulative("2026-11-05T00:01:00+08:00", 100)+
		cumulative("2026-11-05T00:02:00+08:00", 125)+
		cumulative("2026-11-05T00:03:00+08:00", 20)+
		cumulative("2026-11-05T00:04:00+08:00", 33))
	index := usage.NewIndex(root)
	snapshot := index.Scan(testTime(t, "2026-11-05T00:30:00+08:00"), false)
	assertUsage(t, snapshot, "38", 2, usage.PartialError)
	if snapshot.Message == nil || snapshot.Breakdown.CacheHitRatePercent != nil {
		t.Fatalf("expected a visible gap, got %+v", snapshot)
	}
}

func TestArchiveMovePreservesUsage(t *testing.T) {
	home := t.TempDir()
	active := filepath.Join(home, "sessions")
	archive := filepath.Join(home, "archived_sessions")
	for _, root := range []string{active, archive} {
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	now := testTime(t, "2026-11-05T00:30:00+08:00")
	path := filepath.Join(active, "session.jsonl")
	writeLog(t, path, response("2026-11-05T00:01:00+08:00", "thread", "r1", 10))
	index := usage.NewIndex(active)
	assertUsage(t, index.Scan(now, false), "10", 1, usage.OK)

	if err := os.Rename(path, filepath.Join(archive, filepath.Base(path))); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(active); err != nil {
		t.Fatal(err)
	}
	assertUsage(t, index.Scan(now, false), "10", 1, usage.OK)
}

func TestBreakdownUsesWeightedCacheRateWithTwoDecimals(t *testing.T) {
	root := t.TempDir()
	today := "2026-11-05T00:01:00+08:00"
	writeLog(t, filepath.Join(root, "session.jsonl"),
		responseWithUsage(today, "thread", "r1", `"total_tokens":600,"input_tokens":100,"cached_input_tokens":0,"cache_write_input_tokens":20,"output_tokens":500,"reasoning_output_tokens":40`)+
			responseWithUsage(today, "thread", "r2", `"total_tokens":19907,"input_tokens":19900,"cached_input_tokens":19289,"cache_write_input_tokens":0,"output_tokens":7,"reasoning_output_tokens":1`))
	snapshot := usage.NewIndex(root).Scan(testTime(t, "2026-11-05T00:30:00+08:00"), false)
	assertUsage(t, snapshot, "20507", 2, usage.OK)
	b := snapshot.Breakdown
	for _, field := range []struct {
		name string
		got  *string
		want string
	}{
		{"input", b.InputTokens, "20000"}, {"cached", b.CachedInputTokens, "19289"},
		{"uncached", b.UncachedInputTokens, "711"}, {"cache writes", b.CacheWriteInputTokens, "20"},
		{"output", b.OutputTokens, "507"}, {"reasoning", b.ReasoningOutputTokens, "41"},
		{"non-reasoning", b.NonReasoningOutputTokens, "466"},
		{"cache hit rate", b.CacheHitRatePercent, "96.45"},
	} {
		if field.got == nil || *field.got != field.want {
			t.Errorf("%s must be %s", field.name, field.want)
		}
	}
}

func TestMissingCacheDataDoesNotBecomeZero(t *testing.T) {
	root := t.TempDir()
	writeLog(t, filepath.Join(root, "session.jsonl"), responseWithUsage(
		"2026-11-05T00:01:00+08:00", "thread", "r1",
		`"total_tokens":105,"input_tokens":100,"cache_write_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":1`))
	snapshot := usage.NewIndex(root).Scan(testTime(t, "2026-11-05T00:30:00+08:00"), false)
	assertUsage(t, snapshot, "105", 1, usage.PartialError)
	b := snapshot.Breakdown
	if b.CachedInputTokens != nil || b.UncachedInputTokens != nil || b.CacheHitRatePercent != nil ||
		b.OutputTokens == nil || *b.OutputTokens != "5" || snapshot.Message == nil {
		t.Fatalf("unexpected breakdown: %+v", b)
	}
}

func TestOutputOnlyUsageHasNoCacheRate(t *testing.T) {
	root := t.TempDir()
	writeLog(t, filepath.Join(root, "session.jsonl"), responseWithUsage(
		"2026-11-05T00:01:00+08:00", "thread", "r1",
		`"total_tokens":5,"input_tokens":0,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":1`))
	snapshot := usage.NewIndex(root).Scan(testTime(t, "2026-11-05T00:30:00+08:00"), false)
	assertUsage(t, snapshot, "5", 1, usage.OK)
	if snapshot.Breakdown.CacheHitRatePercent != nil {
		t.Fatal("zero input has no cache hit rate")
	}
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

func metadata(thread string) string {
	return fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q}}`+"\n", thread)
}

func response(timestamp, thread, id string, tokens uint64) string {
	return responseWithUsage(timestamp, thread, id, fmt.Sprintf(
		`"total_tokens":%d,"input_tokens":%d,"cached_input_tokens":0,"cache_write_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0`, tokens, tokens))
}

func responseWithUsage(timestamp, thread, id, fields string) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"token_usage_record","payload":{"thread_id":%q,"response_id":%q,"usage":{%s}}}`+"\n",
		timestamp, thread, id, fields)
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
