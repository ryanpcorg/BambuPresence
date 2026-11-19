package main

import (
	"fmt"
	"os"

	"bambupresence/internal/config"
	"bambupresence/internal/ui"
)

func main() {
	cfg, err := config.Load(config.DefaultConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
	}

	app := ui.NewApp(cfg)
	app.Run()
}
