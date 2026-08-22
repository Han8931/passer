package main

import (
	"fmt"
	"os"

	"passer/internal/clip"
	"passer/internal/config"
	"passer/internal/pass"
	"passer/internal/ui"
)

func main() {
	configPath := config.ResolveConfigPath()
	cfg, err := config.LoadOrCreate(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	store, err := pass.Open(cfg.StoreDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	// A missing clipboard tool is not fatal: browsing, revealing, and editing
	// all still work, so we start anyway and report the problem when a copy is
	// actually attempted.
	board, clipErr := clip.Detect()

	if err := ui.Run(store, board, clipErr, cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "error running passer: %v\n", err)
		os.Exit(1)
	}
}
