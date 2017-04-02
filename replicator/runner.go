package replicator

import (
	"time"

	"github.com/elsevier-core-engineering/replicator/api"
	"github.com/elsevier-core-engineering/replicator/logging"
	"github.com/elsevier-core-engineering/replicator/replicator/structs"
)

// Runner is the main runner struct.
type Runner struct {
	// doneChan is where finish notifications occur.
	doneChan chan struct{}

	// config is the Config that created this Runner. It is used internally to
	// construct other objects and pass data.
	config *structs.Config
}

// NewRunner sets up the Runner type.
func NewRunner(config *structs.Config) (*Runner, error) {
	runner := &Runner{
		doneChan: make(chan struct{}),
		config:   config,
	}
	return runner, nil
}

// Start creates a new runner and uses a ticker to block until the doneChan is
// closed at which point the ticker is stopped.
func (r *Runner) Start() {
	ticker := time.NewTicker(time.Second * time.Duration(5))

	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:

			clusterChan := make(chan bool)
			go r.clusterScaling(clusterChan)
			<-clusterChan

			r.jobScaling()

			// TODO: Consolidate cluster scaling into single entry-point method that can
			// be called concurrently. This includes the following:
			// - ClusterAllocationCapacity
			// - ClusterAssignedAllocation
			// - TaskAllocationTotals
			// - api.PercentageCapacityRequired
			// - MostUtilizedResource
			// - CheckClusterScalingTimeThreshold (only required for scaling operations)
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
			//
			// target := client.LeastAllocatedNode(allocs)
			// logging.Info("Least Allocated Node: %v", target)
			// // client.DrainNode(target)
			logging.Info("%v", RuntimeStats())
		case <-r.doneChan:
			return
		}
	}
}

// Stop halts the execution of this runner.
func (r *Runner) Stop() {
	close(r.doneChan)
}

// clusterScaling is the main entry point into the cluster scaling functionality
// and ties numerous functions together to create an asynchronus function which
// can be called from the runner.
func (r *Runner) clusterScaling(done chan bool) {
	client := r.config.NomadClient
	scalingEnabled := r.config.ClusterScaling.Enabled

	if r.config.Region == "" {
		if region, err := api.DescribeAWSRegion(); err == nil {
			r.config.Region = region
		}
	}

	clusterCapacity := &structs.ClusterAllocation{}

	if scale, err := client.EvaluateClusterCapacity(clusterCapacity, r.config); err != nil || !scale {
		logging.Info("scaling operation not permitted")
	} else {
		// If we reached this point we will be performning AWS interaction so we
		// create an client connection.
		asgSess := api.NewAWSAsgService(r.config.Region)

		if clusterCapacity.ScalingDirection == api.ScalingDirectionOut {
			if !scalingEnabled {
				logging.Info("cluster scaling disabled, not initiating scaling operation (scale-out)")
				done <- true
				return
			}

			clusterCapacity.LastScalingEvent = time.Now()

			if err := api.ScaleOutCluster(r.config.ClusterScaling.AutoscalingGroup, asgSess); err != nil {
				logging.Error("unable to successfully scale out cluster: %v", err)
			}
		}

		if clusterCapacity.ScalingDirection == api.ScalingDirectionIn {
			nodeID, nodeIP := client.LeastAllocatedNode(clusterCapacity)
			if nodeIP != "" && nodeID != "" {
				logging.Info("NodeIP: %v, NodeID: %v", nodeIP, nodeID)
				if !scalingEnabled {
					logging.Info("cluster scaling disabled, not initiating scaling operation (scale-in)")
					done <- true
					return
				}
				if err := client.DrainNode(nodeID); err == nil {
					logging.Info("terminating AWS instance %v", nodeIP)
					err := api.ScaleInCluster(r.config.ClusterScaling.AutoscalingGroup, nodeIP, asgSess)
					if err != nil {
						logging.Error("unable to successfully terminate AWS instance %v: %v", nodeID, err)
					}
				}
			}
		}
	}
	done <- true
	return
}

// jobScaling is the main entry point for the Nomad job scaling functionality
// and ties together a number of functions to be called from the runner.
func (r *Runner) jobScaling() {

	// Scaling a Cluster Jobs requires access to both Consul and Nomad therefore
	// we setup the clients here.
	consulClient := r.config.ConsulClient

	nomadClient := r.config.NomadClient

	// Pull the list of all currently running jobs which have an enabled scaling
	// document.
	resp, err := consulClient.ListConsulKV(r.config, nomadClient)
	if err != nil {
		logging.Error("%v", err)
	}

	// EvaluateJobScaling identifies whether each of the Job.Groups requires a
	// scaling event to be triggered. This is then iterated so the individual
	// groups can be assesed.
	nomadClient.EvaluateJobScaling(resp)
	for _, job := range resp {

		// Due to the nested nature of the job and group Nomad definitions a dumb
		// metric is used to determine whether the job has 1 or more groups which
		// require scaling.
		i := 0

		for _, group := range job.GroupScalingPolicies {
			if group.Scaling.ScaleDirection == "Out" || group.Scaling.ScaleDirection == "In" {
				logging.Info("scale %v to be requested on job \"%v\" and group \"%v\"", group.Scaling.ScaleDirection, job.JobName, group.GroupName)
				i++
			}
		}

		// If 1 or more groups need to be scaled we submit the whole job for scaling
		// as to scale you must submit the whole job file currently. The JobScale
		// function takes care of scaling groups independently.
		if i > 0 {
			nomadClient.JobScale(job)
		}
	}
}
