package replicator

import (
	"time"

	"github.com/elsevier-core-engineering/replicator/api"
	"github.com/elsevier-core-engineering/replicator/logging"
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
	ticker := time.NewTicker(time.Second * time.Duration(1))

	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			client, _ := api.NewNomadClient(r.config.Nomad)
			allocs := &api.ClusterAllocation{}

			client.ClusterAllocationCapacity(allocs)
			client.ClusterAssignedAllocation(allocs)

			for _, nodeAllocs := range allocs.NodeAllocations {
				logging.Info("Node ID: %v, CPU Percent: %v", nodeAllocs.NodeID, nodeAllocs.UsedCapacity.CPUPercent)
				logging.Info("Node ID: %v, Mem Percent: %v", nodeAllocs.NodeID, nodeAllocs.UsedCapacity.MemoryPercent)
				logging.Info("Node ID: %v, Disk Percent: %v", nodeAllocs.NodeID, nodeAllocs.UsedCapacity.DiskPercent)
			}

			client.TaskAllocationTotals(allocs)
			client.MostUtilizedResource(allocs)
			logging.Info("Scaling Metric: %v", allocs.ScalingMetric)

			res := api.PercentageCapacityRequired(allocs.NodeCount, allocs.TaskAllocation.CPUMHz, allocs.TotalCapacity.CPUMHz, allocs.UsedCapacity.CPUMHz, 2)
			logging.Info("precentage cluster capactity required: %v", res)

			logging.Info("Node Count: %v", allocs.NodeCount)
			logging.Info("CPU: %v %v", allocs.UsedCapacity.CPUMHz, allocs.TotalCapacity.CPUMHz)
			logging.Info("Memory: %v %v", allocs.UsedCapacity.MemoryMB, allocs.TotalCapacity.MemoryMB)
			logging.Info("Disk: %v %v", allocs.UsedCapacity.DiskMB, allocs.TotalCapacity.DiskMB)
			if client.LeaderCheck() {
				logging.Info("We have cluster leadership.")
			}

			target := client.LeastAllocatedNode(allocs)
			logging.Info("Least Allocated Node: %v", target)
			// client.DrainNode(target)
		case <-r.doneChan:
			return
		}
	}
}

// Stop halts the execution of this runner.
func (r *Runner) Stop() {
	close(r.doneChan)
}
