package replicator

import (
	"fmt"
	"time"

	"github.com/elsevier-core-engineering/replicator/api"
)

// Runner is the main runner struct.
type Runner struct {
	// doneChan is where finish notifications occur.
	doneChan chan struct{}

	// config is the Config that created this Runner. It is used internally to
	// construct other objects and pass data.
	config *Config
}

// NewRunner sets up the Runner type.
func NewRunner(config *Config) (*Runner, error) {
	runner := &Runner{
		doneChan: make(chan struct{}),
		config:   config,
	}
	return runner, nil
}

// Start creates a new runner and uses a ticker to block until the doneChan is
// closed at which point the ticker is stopped.
func (r *Runner) Start() {
	ticker := time.NewTicker(time.Second * time.Duration(10))

	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			client, _ := api.NewNomadClient(r.config.Nomad)
			resp, _ := client.AllocationCapacity()
			res, _ := client.AssignedAllocation()
			fmt.Println(resp.CPU, res.CPU)
			fmt.Println(resp.MemoryMB, res.MemoryMB)
			fmt.Println(resp.DiskMB, res.DiskMB)
		case <-r.doneChan:
			return
		}
	}
}

// Stop halts the execution of this runner.
func (r *Runner) Stop() {
	close(r.doneChan)
}
