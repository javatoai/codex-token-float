package usage

import (
	"maps"
	"slices"
)

type fileSummary struct {
	threadID   string
	structured bool
	responses  []logRecord
	legacy     map[string]tally
}

func summarizeSession(records []logRecord) *fileSummary {
	summary := &fileSummary{}
	var forked bool
	for _, record := range records {
		if record.kind == sessionMeta && summary.threadID == "" {
			summary.threadID, forked = record.threadID, record.forked
		}
		summary.structured = summary.structured || record.kind == responseUsage
	}
	if summary.structured {
		for _, record := range records {
			if record.kind == responseUsage &&
				(summary.threadID == "" || record.threadID == "" || record.threadID == summary.threadID) {
				summary.responses = append(summary.responses, record)
			}
		}
		return summary
	}

	var previous tokenUsage
	for _, record := range records {
		if record.kind != tokenCount {
			continue
		}
		day := record.day()
		daily := summary.legacy[day]
		current := record.usage
		switch {
		case current.Total == nil || forked:
			// Legacy copies carry cumulative snapshots without response IDs.
			daily.incomplete = true
		case previous.Total == nil:
			daily.incomplete = daily.incomplete || *current.Total > 0
		case *current.Total < *previous.Total:
			daily.incomplete = true
		default:
			daily.addUsage(current.difference(previous))
		}
		if summary.legacy == nil {
			summary.legacy = make(map[string]tally)
		}
		summary.legacy[day] = daily
		if current.Total != nil {
			previous = current
		}
	}
	return summary
}

func indexDays(files map[string]session) map[string]tally {
	days := make(map[string]tally)
	responses := make(map[string]logRecord)
	structured := make(map[string]bool)
	legacy := make(map[string]session)
	for _, path := range slices.Sorted(maps.Keys(files)) {
		file := files[path]
		summary := file.summary
		if summary == nil {
			continue
		}
		key := summary.threadID
		if key == "" {
			key = path
		}
		if summary.structured {
			structured[key] = true
			for _, record := range summary.responses {
				if record.responseID == "" || record.usage.Total == nil {
					day := record.day()
					daily := days[day]
					daily.incomplete = true
					days[day] = daily
					continue
				}
				previous, exists := responses[record.responseID]
				if !exists || record.at.Before(previous.at) {
					responses[record.responseID] = record
				}
			}
		} else if prior, exists := legacy[key]; !exists || newerLog(file, prior) {
			legacy[key] = file
		}
	}
	for _, record := range responses {
		day := record.day()
		daily := days[day]
		daily.addUsage(record.usage)
		days[day] = daily
	}
	for key, file := range legacy {
		if structured[key] {
			continue
		}
		for day, tally := range file.summary.legacy {
			daily := days[day]
			daily.add(tally)
			days[day] = daily
		}
	}
	return days
}

func newerLog(file, prior session) bool {
	modified, previous := file.info.ModTime(), prior.info.ModTime()
	return modified.After(previous) || (modified.Equal(previous) && file.info.Size() > prior.info.Size())
}
