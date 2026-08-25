// Command apiledger verifies the release migration ledger against published Go APIs.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/Tangerg/oolong/internal/apiledger"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg := apiledger.Config{Log: os.Stdout}
	flag.StringVar(&cfg.Root, "root", ".", "repository root")
	flag.StringVar(&cfg.Section, "section", "Unreleased", "CHANGELOG release section")
	flag.StringVar(&cfg.APIDiff, "apidiff", "apidiff", "apidiff executable")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := apiledger.Check(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
