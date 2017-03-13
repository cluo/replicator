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
			allocs := &api.ClusterAllocation{}

			client.ClusterAllocationCapacity(allocs)
			client.ClusterAssignedAllocation(allocs)
			client.TaskAllocationTotals(allocs)

			res := api.PercentageCapacityRequired(allocs.NodeCount, allocs.TaskAllocation.CPUMHz, allocs.ClusterTotalAllocationCapacity.CPUMHz, allocs.ClusterUsedAllocationCapacity.CPUMHz, 2)
			fmt.Println(res)

			fmt.Printf("Node Count: %v\n", allocs.NodeCount)
			fmt.Printf("CPU: %v %v\n", allocs.ClusterUsedAllocationCapacity.CPUMHz, allocs.ClusterTotalAllocationCapacity.CPUMHz)
			fmt.Printf("Memory: %v %v\n", allocs.ClusterUsedAllocationCapacity.MemoryMB, allocs.ClusterTotalAllocationCapacity.MemoryMB)
			fmt.Printf("Disk: %v %v\n", allocs.ClusterUsedAllocationCapacity.DiskMB, allocs.ClusterTotalAllocationCapacity.DiskMB)
			if client.LeaderCheck() {
				fmt.Printf("We have cluster leadership.\n")
			}
		case <-r.doneChan:
			return
		}
	}
}

// Stop halts the execution of this runner.
func (r *Runner) Stop() {
	close(r.doneChan)
}
