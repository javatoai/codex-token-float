package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/javatoai/codex-token-float/internal/desktop"
	"github.com/javatoai/codex-token-float/internal/usage"
)

func main() {
	printToday := flag.Bool("print-today", false, "输出北京时间今日统计的 JSON")
	flag.Parse()

	root := usage.SessionsDir()
	var err error
	if *printToday {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(usage.NewIndex(root).Scan(time.Now(), false))
	} else {
		err = desktop.Run(root)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
