package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/javatoai/codex-token-float/assets"
	"github.com/javatoai/codex-token-float/internal/desktop"
	"github.com/javatoai/codex-token-float/internal/usage"
)

func main() {
	printToday := flag.Bool("print-today", false, "输出北京时间今日统计的 JSON")
	licenses := flag.Bool("licenses", false, "查看应用和依赖许可")
	flag.Parse()
	if *licenses {
		fmt.Print(assets.Licenses)
		return
	}

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
