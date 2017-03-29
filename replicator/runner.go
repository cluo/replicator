package replicator

import (
	"time"

	"github.com/elsevier-core-engineering/replicator/api"
	config "github.com/elsevier-core-engineering/replicator/config/structs"
	"github.com/elsevier-core-engineering/replicator/logging"
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
			//consulClient, _ := api.NewConsulClient(r.config.Consul)

			clusterCapacity := &api.ClusterAllocation{}
			client.EvaluateClusterCapacity(clusterCapacity, r.config)

			logging.Info("Cluster Capacity: CPU - %v, MEM - %v", clusterCapacity.TotalCapacity.CPUMHz,
				clusterCapacity.TotalCapacity.MemoryMB)
			logging.Info("Cluster Usage: CPU - %v, MEM - %v", clusterCapacity.UsedCapacity.CPUPercent,
				clusterCapacity.UsedCapacity.MemoryPercent)
			logging.Info("Scaling Metric: %v", clusterCapacity.ScalingMetric)

			// TODO: Consolidate cluster scaling into single entry-point method that can
			// be called concurrently. This includes the following:
			// - ClusterAllocationCapacity
			// - ClusterAssignedAllocation
			// - TaskAllocationTotals
			// - api.PercentageCapacityRequired
			// - MostUtilizedResource
			// - CheckClusterScalingTimeThreshold
			// - LeastAllocatedNode (Only required for scale-in operations)
			// - DrainNode (Only required for scale-in operations)
			// - ScaleOutCluster, ScaleInCluster

			// client.ClusterAllocationCapacity(allocs)
			// client.ClusterAssignedAllocation(allocs)
			//
			// for _, nodeAllocs := range allocs.NodeAllocations {
			// 	logging.Info("Node ID: %v, CPU Percent: %v", nodeAllocs.NodeID, nodeAllocs.UsedCapacity.CPUPercent)
			// 	logging.Info("Node ID: %v, Mem Percent: %v", nodeAllocs.NodeID, nodeAllocs.UsedCapacity.MemoryPercent)
			// 	logging.Info("Node ID: %v, Disk Percent: %v", nodeAllocs.NodeID, nodeAllocs.UsedCapacity.DiskPercent)
			// }
			//
			// client.TaskAllocationTotals(allocs)
			// client.MostUtilizedResource(allocs)
			// logging.Info("Scaling Metric: %v", allocs.ScalingMetric)
			//
			// res := api.PercentageCapacityRequired(allocs.NodeCount, allocs.TaskAllocation.CPUMHz, allocs.TotalCapacity.CPUMHz, allocs.UsedCapacity.CPUMHz, 2)
			// logging.Info("precentage cluster capactity required: %v", res)
			//
			// logging.Info("Node Count: %v", allocs.NodeCount)
			// logging.Info("CPU: %v %v", allocs.UsedCapacity.CPUMHz, allocs.TotalCapacity.CPUMHz)
			// logging.Info("Memory: %v %v", allocs.UsedCapacity.MemoryMB, allocs.TotalCapacity.MemoryMB)
			// logging.Info("Disk: %v %v", allocs.UsedCapacity.DiskMB, allocs.TotalCapacity.DiskMB)
			//
			// // TODO: Move this check to the beginning and halt execution for this cycle if we do not
			// // have cluster leadership.
			// if client.LeaderCheck() {
			// 	logging.Info("We have cluster leadership.")
			// }
			//
			// // TODO: Consolidate job scaling into single entry point; this includes:
			// // - consulClient.ListConsulKV (returns only running jobs and those enabled for scaling)
			// // - EvaluateJobScaling
			// // - JobScale
			//
			// scalingPolicies, _ := consulClient.ListConsulKV("", "replicator/config/jobs", r.config)
			// logging.Info("%v", scalingPolicies)
			// client.EvaluateJobScaling(scalingPolicies)
			//
			// for _, sp := range scalingPolicies {
			// 	for _, gsp := range sp.GroupScalingPolicies {
			// 		logging.Info("Group Name: %v, Scaling Metric: %v", gsp.GroupName, gsp.ScalingMetric)
			// 		logging.Info("Group Name: %v, CPU: %v, Memory: %v", gsp.GroupName, gsp.Tasks.Resources.CPUPercent,
			// 			gsp.Tasks.Resources.MemoryPercent)
			// 		logging.Info("Group Name: %v, Scaling Direction: %v", gsp.GroupName, gsp.Scaling.ScaleDirection)
			// 	}
			// }
			//
			// target := client.LeastAllocatedNode(allocs)
			// logging.Info("Least Allocated Node: %v", target)
			// // client.DrainNode(target)
		case <-r.doneChan:
			return
		}
	}
}

// Stop halts the execution of this runner.
func (r *Runner) Stop() {
	close(r.doneChan)
}
