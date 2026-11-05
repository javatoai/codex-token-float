package usage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var beijing = time.FixedZone("UTC+8", 8*60*60)

type Status string

const (
	OK               Status = "ok"
	NoData           Status = "no_data"
	MissingDirectory Status = "missing_directory"
	PartialError     Status = "partial_error"
)

type Snapshot struct {
	Date           string    `json:"date"`
	TotalTokens    string    `json:"totalTokens"`
	FormattedTotal string    `json:"formattedTotal"`
	FileCount      int       `json:"fileCount"`
	EventCount     uint64    `json:"eventCount"`
	UpdatedAt      string    `json:"updatedAt"`
	Status         Status    `json:"status"`
	Message        *string   `json:"message"`
	Breakdown      Breakdown `json:"breakdown"`
}

func SessionsDir() string {
	if home, ok := os.LookupEnv("CODEX_HOME"); ok {
		return filepath.Join(home, "sessions")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "sessions")
}

type Index struct {
	root  string
	files map[string]session
	days  map[string]tally
}

func NewIndex(root string) *Index {
	return &Index{root: root}
}

// Scan updates the index and returns a snapshot. A single goroutine owns the index.
func (i *Index) Scan(now time.Time, rebuild bool) Snapshot {
	now = now.In(beijing)
	date := now.Format(time.DateOnly)
	files := make(map[string]session, len(i.files))
	active, activeErrors := i.scanRoot(i.root, files, rebuild)
	archiveRoot := filepath.Join(filepath.Dir(i.root), "archived_sessions")
	archived, archiveErrors := i.scanRoot(archiveRoot, files, rebuild)
	if i.days == nil || filesChanged(i.files, files) {
		i.days = indexDays(files)
	}
	i.files = files

	daily := i.days[date]
	total := strconv.FormatUint(daily.tokens, 10)
	snapshot := Snapshot{
		Date: date, UpdatedAt: now.Format(time.TimeOnly),
		TotalTokens: total, FormattedTotal: FormatTokens(total),
		FileCount: len(files), EventCount: daily.events, Status: NoData,
	}
	var message string
	failed := activeErrors + archiveErrors
	switch {
	case failed > 0:
		daily.incomplete = true
		snapshot.Status = PartialError
		message = fmt.Sprintf("%d 处日志暂时无法读取，统计有缺口", failed)
	case !active && !archived:
		daily.incomplete = true
		snapshot.Status = MissingDirectory
		message = "未找到 Codex 会话日志目录"
	case daily.incomplete:
		snapshot.Status = PartialError
		message = "部分用量缺少可靠记录，显示已统计总量"
	case daily.missingDetails():
		snapshot.Status = PartialError
		message = "部分用量缺少分类明细"
	case daily.events > 0:
		snapshot.Status = OK
	}
	if message != "" {
		snapshot.Message = &message
	}
	snapshot.Breakdown = daily.breakdown()
	return snapshot
}

func (i *Index) scanRoot(root string, files map[string]session, rebuild bool) (bool, int) {
	var available bool
	var failed int
	var unreadableDirs []string
	fs.WalkDir(os.DirFS(root), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if relative == "." && (errors.Is(walkErr, fs.ErrNotExist) || (walkErr == nil && !entry.IsDir())) {
			return fs.SkipAll
		}
		path := filepath.Join(root, filepath.FromSlash(relative))
		if walkErr != nil {
			if !errors.Is(walkErr, fs.ErrNotExist) {
				failed++
				unreadableDirs = append(unreadableDirs, path)
			}
			return nil
		}
		available = true
		if !entry.Type().IsRegular() || filepath.Ext(relative) != ".jsonl" {
			return nil
		}
		previous, known := i.files[path]
		info, err := entry.Info()
		if err == nil {
			if !known && !rebuild {
				previous = i.findMovedFile(info)
			}
			var updated session
			updated, err = updateSession(path, info, previous, rebuild)
			if err == nil {
				files[path] = updated
				return nil
			}
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		failed++
		previous.retry = true
		files[path] = previous
		return nil
	})
	// Keep the last known contributions under a directory we could not inspect.
	for _, directory := range unreadableDirs {
		prefix := directory + string(filepath.Separator)
		for path, previous := range i.files {
			if _, seen := files[path]; !seen && strings.HasPrefix(path, prefix) {
				previous.retry = previous.retry || rebuild
				files[path] = previous
			}
		}
	}
	return available, failed
}

func filesChanged(previous, current map[string]session) bool {
	if len(previous) != len(current) {
		return true
	}
	for path, file := range current {
		if old, exists := previous[path]; !exists || old.summary != file.summary {
			return true
		}
	}
	return false
}

func (i *Index) findMovedFile(info fs.FileInfo) session {
	for _, file := range i.files {
		if sameFileVersion(file.info, info) {
			return file
		}
	}
	return session{}
}

func sameFileVersion(previous, current fs.FileInfo) bool {
	return previous != nil && current.Size() == previous.Size() &&
		current.ModTime().Equal(previous.ModTime()) && os.SameFile(previous, current)
}

func updateSession(path string, info fs.FileInfo, previous session, rebuild bool) (session, error) {
	if rebuild || previous.retry {
		previous = session{}
	}
	// On Windows, a directory entry can report the old size while a writer
	// keeps the file open. Verify the version using the open file there.
	if runtime.GOOS != "windows" && sameFileVersion(previous.info, info) {
		return previous, nil
	}
	return scanFile(path, previous)
}

func FormatTokens(digits string) string {
	var result strings.Builder
	for index, digit := range digits {
		if index > 0 && (len(digits)-index)%4 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(digit)
	}
	return result.String()
}
