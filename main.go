package main

import (
	"flag"
	"os"

	"github.com/elsevier-core-engineering/replicator/replicator"
)

func init() {
	// Setup the config CLI flag.
	var config string
	flag.StringVar(&config, "config", "", "config file to read overrides")
	flag.Parse()
}

func main() {
	cli := replicator.NewCLI()
	os.Exit(cli.Run(os.Args[1:]))
}
