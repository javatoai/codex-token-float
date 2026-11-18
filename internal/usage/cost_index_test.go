package usage_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/javatoai/codex-token-float/internal/usage"
)

const costFixture = `"total_tokens":1100000,"input_tokens":1000000,"cached_input_tokens":600000,"cache_write_input_tokens":10000,"output_tokens":100000,"reasoning_output_tokens":90000`

func TestAPICostAcrossModelsAndArchivedCopies(t *testing.T) {
	home := t.TempDir()
	active, archive := filepath.Join(home, "sessions"), filepath.Join(home, "archived_sessions")
	for _, directory := range []string{active, archive} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	before, today := "2026-11-04T15:59:59Z", "2026-11-04T16:00:00Z"
	const astraUsage = `"total_tokens":760000,"input_tokens":700000,"cached_input_tokens":200000,"cache_write_input_tokens":25000,"output_tokens":60000,"reasoning_output_tokens":30000`
	context := metadata("thread") + modelContext("sol-turn", "gpt-6.2-sol")
	writeLog(t, filepath.Join(archive, "original.jsonl"), context+
		responseOnTurn(before, "thread", "sol-turn", "yesterday", costFixture)+
		responseOnTurn(today, "thread", "sol-turn", "sol-response", costFixture))
	writeLog(t, filepath.Join(active, "active.jsonl"), context+
		responseOnTurn(today, "thread", "sol-turn", "sol-response", costFixture)+
		responseOnTurn(today, "thread", "astra-turn", "astra-response", astraUsage)+
		modelContext("astra-turn", "gpt-6.2-astra"))

	index := usage.NewIndex(active)
	assertAPICost(t, index.Scan(testTime(t, before), false), "1.86", false)
	snapshot := index.Scan(testTime(t, today), false)
	assertUsage(t, snapshot, "1860000", 2, usage.OK)
	assertAPICost(t, snapshot, "8.34", false)
	details := snapshot.APICostBreakdown
	for _, row := range []struct {
		name string
		cost usage.APICost
		want string
	}{
		{"input", details.Input, "4.94"}, {"cached", details.CachedInput, "0.14"},
		{"uncached", details.UncachedInput, "4.80"}, {"cache write", details.CacheWriteInput, "0.22"},
		{"output", details.Output, "3.40"}, {"reasoning", details.ReasoningOutput, "2.10"},
		{"non-reasoning", details.NonReasoningOutput, "1.30"},
	} {
		t.Run(row.name, func(t *testing.T) { assertCost(t, row.cost, row.want, false) })
	}
	next := index.Scan(testTime(t, today).Add(24*time.Hour), false)
	assertAPICost(t, next, "0.00", false)
	assertCost(t, next.APICostBreakdown.Input, "0.00", false)
}

func TestAPICostUsesKnownSubtotal(t *testing.T) {
	const today = "2026-11-04T16:00:00Z"
	now := testTime(t, today)
	for _, test := range []struct {
		name    string
		context string
		fields  string
	}{
		{"missing model", "", costFixture},
		{"unlisted model", modelContext("turn", "gpt-future"), costFixture},
		{"conflicting model", modelContext("turn", "gpt-6.2-sol") + modelContext("turn", "gpt-6.2-astra"), costFixture},
		{"missing cache count", modelContext("turn", "gpt-6.2-sol"), `"total_tokens":1100000,"input_tokens":1000000,"output_tokens":100000`},
		{"invalid cache count", modelContext("turn", "gpt-6.2-sol"), `"total_tokens":1100000,"input_tokens":1000000,"cached_input_tokens":1100000,"output_tokens":100000`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "sessions")
			index := usage.NewIndex(root)
			assertAPICost(t, index.Scan(now, false), "", true)
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "sample.jsonl")
			writeLog(t, path, metadata("thread")+test.context+responseOnTurn(today, "thread", "turn", "unpriced", test.fields))
			assertAPICost(t, index.Scan(now, false), "", true)
			appendLog(t, path, modelContext("known", "gpt-6.2-sol")+
				responseOnTurn(today, "thread", "known", "priced", costFixture), now)
			snapshot := index.Scan(now, false)
			if snapshot.TotalTokens != "2200000" || snapshot.EventCount != 2 {
				t.Fatalf("both responses must contribute to tokens: %+v", snapshot)
			}
			assertAPICost(t, snapshot, "1.86", true)
			assertCost(t, snapshot.APICostBreakdown.Input, "0.86", true)
			assertCost(t, snapshot.APICostBreakdown.Output, "1.00", true)
		})
	}
}

func modelContext(turn, model string) string {
	return fmt.Sprintf(`{"type":"turn_context","payload":{"turn_id":%q,"model":%q}}`+"\n", turn, model)
}

func responseOnTurn(timestamp, thread, turn, id, fields string) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"token_usage_record","payload":{"thread_id":%q,"turn_id":%q,"response_id":%q,"usage":{%s}}}`+"\n",
		timestamp, thread, turn, id, fields)
}

func assertAPICost(t *testing.T, snapshot usage.Snapshot, want string, incomplete bool) {
	t.Helper()
	assertCost(t, snapshot.APICost, want, incomplete)
}

func assertCost(t *testing.T, cost usage.APICost, want string, incomplete bool) {
	t.Helper()
	var got string
	if cost.USD != nil {
		got = *cost.USD
	}
	if got != want || cost.Incomplete != incomplete {
		t.Fatalf("cost = %q (incomplete %v), want %q (incomplete %v)", got, cost.Incomplete, want, incomplete)
	}
}
