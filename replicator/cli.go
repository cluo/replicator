package replicator

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/elsevier-core-engineering/replicator/config"
	"github.com/elsevier-core-engineering/replicator/config/structs"
)

// Setup our exit codes; errors start at 10 for easier debugging.
const (
	ExitCodeOK int = 0

	ExitCodeError = 10 + iota
	ExitCodeRunnerError
	ExitCodeInterrupt
	ExitCodeParseFlagsError
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
		return ExitCodeParseFlagsError
	}

	// Create the initial runner with the merged configuration parameters.
	runner, err := NewRunner(c)
	if err != nil {
		return ExitCodeRunnerError
	}

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
					return ExitCodeParseFlagsError
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

	file := "/etc/consulate.d/consulate.hcl"
	var c *structs.Config
	var err error

	// When no command line arguments are passed the default configuration file
	// is initially checked for existance
	if len(args) == 0 {
		if _, er := os.Stat(file); os.IsNotExist(er) {
			return config.DefaultConfig(), nil
		}
		c, err = config.ParseConfig(file)
		if err != nil {
			return nil, fmt.Errorf("%v", err)
		}
		return c, nil
	}

	// If the length of the CLI args is greater than one then there is an error.
	if len(args) > 1 {
		return nil, fmt.Errorf("too many command line args\n %v", usage)
	}

	// If one CLI argument is passed this is split using the equals delimiter and
	// the right hand side used as the configuration file to parse.
	if len(args) == 1 {
		s := strings.Split(args[0], "=")
		c, err = config.ParseConfig(s[1])

		if err != nil {
			return nil, fmt.Errorf("%v", err)
		}
	}
	return c, nil
}

const usage = `
Usage: replicator [options]

Options:
  -config=<path>
      Sets the path to a configuration file on disk. This should be the full
      path rather than relative path.
`
