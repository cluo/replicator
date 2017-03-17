package replicator

import (
	"fmt"
	"time"

	"github.com/elsevier-core-engineering/replicator/api"
	config "github.com/elsevier-core-engineering/replicator/config/structs"
)

// Runner is the main runner struct.
type Runner struct {
	// doneChan is where finish notifications occur.
	doneChan chan struct{}

	// config is the Config that created this Runner. It is used internally to
	// construct other objects and pass data.
	config *config.Config
}

// NewRunner sets up the Runner type.
func NewRunner(config *config.Config) (*Runner, error) {
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
			consulClient, _ := api.NewConsulClient(r.config.Consul)

			allocs := &api.ClusterAllocation{}

			client.ClusterAllocationCapacity(allocs)
			client.ClusterAssignedAllocation(allocs)

			for _, nodeAllocs := range allocs.NodeAllocations {
				fmt.Printf("Node ID: %v, CPU Percent: %v\n", nodeAllocs.NodeID, nodeAllocs.UsedCapacity.CPUPercent)
				fmt.Printf("Node ID: %v, Mem Percent: %v\n", nodeAllocs.NodeID, nodeAllocs.UsedCapacity.MemoryPercent)
				fmt.Printf("Node ID: %v, Disk Percent: %v\n", nodeAllocs.NodeID, nodeAllocs.UsedCapacity.DiskPercent)
			}

			client.TaskAllocationTotals(allocs)
			client.MostUtilizedResource(allocs)
			fmt.Printf("Scaling Metric: %v\n", allocs.ScalingMetric)

			res := api.PercentageCapacityRequired(allocs.NodeCount, allocs.TaskAllocation.CPUMHz, allocs.TotalCapacity.CPUMHz, allocs.UsedCapacity.CPUMHz, 2)
			fmt.Println(res)

			fmt.Printf("Node Count: %v\n", allocs.NodeCount)
			fmt.Printf("CPU: %v %v\n", allocs.UsedCapacity.CPUMHz, allocs.TotalCapacity.CPUMHz)
			fmt.Printf("Memory: %v %v\n", allocs.UsedCapacity.MemoryMB, allocs.TotalCapacity.MemoryMB)
			fmt.Printf("Disk: %v %v\n", allocs.UsedCapacity.DiskMB, allocs.TotalCapacity.DiskMB)
			if client.LeaderCheck() {
				fmt.Printf("We have cluster leadership.\n")
			}

			scalingPolicies, _ := consulClient.ListConsulKV("", "replicator/config/jobs", r.config)

			for _, policy := range scalingPolicies {
				fmt.Println(policy.JobName)
				fmt.Println(len(policy.GroupScalingPolicies))
				for _, groupPolicy := range policy.GroupScalingPolicies {
					fmt.Println(groupPolicy.Scaling.Max)
				}
			}

			target := client.LeastAllocatedNode(allocs)
			fmt.Printf("Least Allocated Node: %v\n", target)
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
