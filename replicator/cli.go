package replicator

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	metrics "github.com/armon/go-metrics"
	"github.com/elsevier-core-engineering/replicator/config"
	"github.com/elsevier-core-engineering/replicator/logging"
	"github.com/elsevier-core-engineering/replicator/replicator/structs"
	"github.com/elsevier-core-engineering/replicator/version"
)

// Setup our exit codes; errors start at 10 for easier debugging.
const (
	ExitCodeOK int = 0

	ExitCodeError = 10 + iota
	ExitCodeRunnerError
	ExitCodeInterrupt
	ExitCodeParseConfigError
	ExitCodeTelemtryError
)

// CLI is the main entry point for Consulate.
type CLI struct {
}

// NewCLI returns the CLI struct.
func NewCLI() *CLI {
	return &CLI{}
}

// Run accepts a slice of command line arguments and ultimatly returns an int
// which represents the exit code of the command. The function sets up the
// runner and appropriate configuration and attempts to perform the run.
func (cli *CLI) Run(args []string) int {

	c, err := cli.setup(args)
	if err != nil {
		logging.Error("unable to parse configuration: %v", err)
		return ExitCodeParseConfigError
	}

	// Set the logging level for the logger.
	logging.SetLevel(c.LogLevel)

	// Initialize telemetry if this was configured by the user.
	if c.Telemetry.StatsdAddress != "" {
		sink, statsErr := metrics.NewStatsdSink(c.Telemetry.StatsdAddress)
		if statsErr != nil {
			logging.Error("unable to setup telemetry correctly: %v", statsErr)
			return ExitCodeTelemtryError
		}
		metrics.NewGlobal(metrics.DefaultConfig("replicator"), sink)
	}

	// Create the initial runner with the merged configuration parameters.
	runner, err := NewRunner(c)
	if err != nil {
		return ExitCodeRunnerError
	}

	logging.Debug("running version %v", version.GetHumanVersion())
	go runner.Start()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	for {
		select {
		case s := <-signalCh:
			switch s {
			case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				runner.Stop()
				return ExitCodeInterrupt

			case syscall.SIGHUP:
				runner.Stop()

				// Reload the configuration in order to make proper use of SIGHUP.
				c, err := cli.setup(args)
				if err != nil {
					return ExitCodeParseConfigError
				}

				// Setup a new runner with the new configuration.
				runner, err = NewRunner(c)
				if err != nil {
					return ExitCodeRunnerError
				}

				go runner.Start()
			}
		}
	}
}

// setup asseses the CLI arguments, or lack of, and then iterates through
// the load order sementics for the configuration to return a configuration
// object.
func (cli *CLI) setup(args []string) (*structs.Config, error) {

	// If the length of the CLI args is greater than one then there is an error.
	if len(args) > 1 {
		return nil, fmt.Errorf("too many command line args\n %v", usage)
	}

	// If no cli flags are passed then we just return a default configuration
	// struct for use.
	if len(args) == 0 {
		return config.DefaultConfig(), nil
	}

	// If one CLI argument is passed this is split using the equals delimiter and
	// the right hand side used as the configuration file/path to parse.
	split := strings.Split(args[0], "=")

	switch p := split[0]; p {
	case "-config":
		c, err := config.FromPath(split[1])
		if err != nil {
			return nil, err
		}
		return c, nil
	default:
		return nil, fmt.Errorf("unable to correctly determine config location %v", split[1])
	}
}

const usage = `
Usage: replicator [options]

Options:
  -config=<path>
      The path to a configuration file or directory of configuration files on
			disk, relative to the current working directory.
`
