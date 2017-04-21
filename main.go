package main

import (
	"flag"
	"os"

	"github.com/elsevier-core-engineering/replicator/replicator"
)

var (
	config string
)

func init() {

	// Setup the config CLI flags.
	flag.StringVar(&config, "config", "", "the relative path to a configuration file or directory")
	flag.Parse()
}

func main() {
	cli := replicator.NewCLI()
	os.Exit(cli.Run(os.Args[1:]))
}
