package api

import (
	"fmt"
	"time"

	"github.com/dariubs/percent"
	nomad "github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
)

// NomadClient exposes all API methods needed to interact with the Nomad API,
// evaluate cluster capacity and allocations and make scaling decisions.
type NomadClient interface {
	// ClusterAllocationCapacity determines the total cluster capacity and current
	// number of worker nodes.
	ClusterAllocationCapacity(*ClusterAllocation) error

	// ClusterAssignedAllocation determines the consumed capacity across the
	// cluster and tracks the resource consumption of each worker node.
	ClusterAssignedAllocation(*ClusterAllocation) error

	// DrainNode places a worker node in drain mode to stop future allocations and
	// migrate existing allocations to other worker nodes.
	DrainNode(string) error

	// LeaderCheck determines if the node running replicator is the gossip pool
	// leader.
	LeaderCheck() bool

	// LeaseAllocatedNode determines the worker node consuming the least amount of
	// the cluster's mosted-utilized resource.
	LeastAllocatedNode(*ClusterAllocation) string

	// MostUtilizedResource calculates which resource is most-utilized across the
	// cluster. The worst-case allocation resource is prioritized when making
	// scaling decisions.
	MostUtilizedResource(*ClusterAllocation)

	// TaskAllocationTotals calculates the allocations required by each running
	// job and what amount of resources required if we increased the count of
	// each job by one. This allows the cluster to proactively ensure it has
	// sufficient capacity for scaling events and deal with potential node failures.
	TaskAllocationTotals(*ClusterAllocation) error
}

// Scaling metric types indicate the most-utilized resource across the cluster. When evaluating
// scaling decisions, the most-utilized resource will be prioritized.
const (
	ScalingMetricNone      = "None" // All supported allocation resources are unutilized.
	ScalingMetricDisk      = "Disk"
	ScalingMetricMemory    = "Memory"
	ScalingMetricProcessor = "CPU"
)

// ClusterAllocation is the central object used to track cluster status and the data
// required to make scaling decisions.
type ClusterAllocation struct {
	// NodeCount is the number of worker nodes in a ready and non-draining state across
	// the cluster.
	NodeCount int

	// ScalingMetric indicates the most-utilized allocation resource across the cluster.
	// The most-utilized resource is prioritized when making scaling decisions like
	// identifying the least-allocated worker node.
	ScalingMetric string

	// ClusterTotalAllocationCapacity is the total allocation capacity across the cluster.
	TotalCapacity AllocationResources

	// ClusterUsedAllocationCapacity is the consumed allocation capacity across the cluster.
	UsedCapacity AllocationResources

	// TaskAllocation represents the total allocation requirements of a single instance
	// (count 1) of all running jobs across the cluster. This is used to practively
	// ensure the cluster has sufficient available capacity to scale each task by +1
	// if an increase in capacity is required.
	TaskAllocation AllocationResources

	// NodeList is a list of all worker nodes in a known good state.
	NodeList []string

	// NodeAllocations is a slice of node allocations.
	NodeAllocations []*NodeAllocation
}

// NodeAllocation describes the resource consumption of a specific worker node.
type NodeAllocation struct {
	// NodeID is the unique ID of the worker node.
	NodeID string

	// UsedCapacity represents the percentage of total cluster resources consumed by
	// the worker node.
	UsedCapacity AllocationResources
}

// AllocationResources represents the allocation resource utilization.
type AllocationResources struct {
	MemoryMB      int
	CPUMHz        int
	DiskMB        int
	MemoryPercent float64
	CPUPercent    float64
	DiskPercent   float64
}

// Provides a wrapper to the Nomad API package.
type nomadClient struct {
	nomad *nomad.Client
}

// NewNomadClient is used to create a new client to interact with Nomad. The
// client implements the NomadClient interface.
func NewNomadClient(addr string) (NomadClient, error) {
	config := nomad.DefaultConfig()
	config.Address = addr
	c, err := nomad.NewClient(config)
	if err != nil {
		return nil, err
	}

	return &nomadClient{nomad: c}, nil
}

// ClusterAllocationCapacity calculates the total cluster capacity and determines the
// number of available worker nodes.
func (c *nomadClient) ClusterAllocationCapacity(capacity *ClusterAllocation) (err error) {
	// Retrieve a list of all worker nodes within the cluster.
	nodes, _, err := c.nomad.Nodes().List(&nomad.QueryOptions{})
	if err != nil {
		return err
	}

	// Get detailed information about each worker node, if the node is in a known-good
	// state, increment the node count, add the node to the node list and add its
	// resources to the overall cluster capacity.
	for _, node := range nodes {
		resp, _, err := c.nomad.Nodes().Info(node.ID, &nomad.QueryOptions{})
		if err != nil {
			return err
		}

		if (resp.Status == "ready") || (resp.Drain != true) {
			capacity.NodeCount++
			capacity.NodeList = append(capacity.NodeList, node.ID)
			capacity.TotalCapacity.CPUMHz += *resp.Resources.CPU
			capacity.TotalCapacity.MemoryMB += *resp.Resources.MemoryMB
			capacity.TotalCapacity.DiskMB += *resp.Resources.DiskMB
		}
	}

	return nil
}

// ClusterAssignedAllocation calculates the total consumed resources across the cluster
// and the amount of resources consumed by each worker node.
func (c *nomadClient) ClusterAssignedAllocation(clusterInfo *ClusterAllocation) (err error) {
	for _, node := range clusterInfo.NodeList {
		allocations, _, err := c.nomad.Nodes().Allocations(node, &nomad.QueryOptions{})
		if err != nil {
			return err
		}

		// Instantiate a new object to track the resource consumption of the worker node.
		nodeInfo := &NodeAllocation{
			NodeID:       node,
			UsedCapacity: AllocationResources{},
		}

		for _, nodeAlloc := range allocations {
			if (nodeAlloc.ClientStatus == "running") && (nodeAlloc.DesiredStatus == "run") {
				// Add the consumed resources to the overall cluster consumed resource values.
				clusterInfo.UsedCapacity.CPUMHz += *nodeAlloc.Resources.CPU
				clusterInfo.UsedCapacity.MemoryMB += *nodeAlloc.Resources.MemoryMB
				clusterInfo.UsedCapacity.DiskMB += *nodeAlloc.Resources.DiskMB

				// Add the consumed resources to the node specific allocation object.
				nodeInfo.UsedCapacity.CPUMHz += *nodeAlloc.Resources.CPU
				nodeInfo.UsedCapacity.MemoryMB += *nodeAlloc.Resources.MemoryMB
				nodeInfo.UsedCapacity.DiskMB += *nodeAlloc.Resources.DiskMB
			}
		}

		// Add the node allocation record to the cluster status object.
		clusterInfo.NodeAllocations = append(clusterInfo.NodeAllocations, nodeInfo)
	}

	// Determine the percentage of overall cluster resources consumed and calculate
	// the amount of those resources consumed by the node.
	CalculateUsage(clusterInfo)

	return
}

// CalculateUsage determines the percentage of overall cluster resources consumed and
// calculates the amount of those resources consumed by each worker node.
func CalculateUsage(clusterInfo *ClusterAllocation) {
	// For each allocation resource, calculate the percentage of overall cluster capacity
	// consumed.
	clusterInfo.UsedCapacity.CPUPercent = percent.PercentOf(
		clusterInfo.UsedCapacity.CPUMHz,
		clusterInfo.TotalCapacity.CPUMHz)

	clusterInfo.UsedCapacity.DiskPercent = percent.PercentOf(
		clusterInfo.UsedCapacity.DiskMB,
		clusterInfo.TotalCapacity.DiskMB)

	clusterInfo.UsedCapacity.MemoryPercent = percent.PercentOf(
		clusterInfo.UsedCapacity.MemoryMB,
		clusterInfo.TotalCapacity.MemoryMB)

	// Determine the amount of consumed resources consumed by each worker node.
	for _, nodeUsage := range clusterInfo.NodeAllocations {
		nodeUsage.UsedCapacity.CPUPercent = percent.PercentOf(nodeUsage.UsedCapacity.CPUMHz,
			clusterInfo.UsedCapacity.CPUMHz)
		fmt.Printf("Node Used: %v (%v), Cluster Used: %v\n", nodeUsage.UsedCapacity.CPUMHz, nodeUsage.UsedCapacity.CPUPercent, clusterInfo.UsedCapacity.CPUMHz)
		nodeUsage.UsedCapacity.DiskPercent = percent.PercentOf(nodeUsage.UsedCapacity.DiskMB,
			clusterInfo.UsedCapacity.DiskMB)
		nodeUsage.UsedCapacity.MemoryPercent = percent.PercentOf(nodeUsage.UsedCapacity.MemoryMB,
			clusterInfo.UsedCapacity.MemoryMB)
	}
}

// LeaderCheck determines if the node running the daemon is the gossip pool leader.
func (c *nomadClient) LeaderCheck() bool {
	haveLeadership := false

	leader, err := c.nomad.Status().Leader()
	if (err != nil) || (len(leader) == 0) {
		fmt.Printf("replicator: failed to identify cluster leader")
	}

	self, err := c.nomad.Agent().Self()
	if err != nil {
		fmt.Printf("replicator: unable to retrieve local agent information")
	} else {

		if FindIP(leader) == self.Member.Addr {
			haveLeadership = true
		}
	}

	return haveLeadership
}

// TaskAllocation determines the total allocation requirements of a single instance (count=1)
// of all running jobs across the cluster. This is used to practively ensure the cluster
// has sufficient available capacity to scale each task by +1 if an increase in capacity
// is required.
func (c *nomadClient) TaskAllocationTotals(capacityUsed *ClusterAllocation) error {
	// Get all jobs across the cluster.
	jobs, _, err := c.nomad.Jobs().List(&nomad.QueryOptions{})
	if err != nil {
		return err
	}

	// Get detailed information about each job.
	for _, job := range jobs {
		resp, _, err := c.nomad.Jobs().Info(job.ID, &nomad.QueryOptions{})
		if err != nil {
			return err
		}

		// A job can contain multiple task groups which can themselves contain multiple tasks;
		// therefore we must iterate fully. The API only returns jobs in a running state.
		for _, taskG := range resp.TaskGroups {
			for _, task := range taskG.Tasks {
				capacityUsed.TaskAllocation.CPUMHz += *task.Resources.CPU
				capacityUsed.TaskAllocation.MemoryMB += *task.Resources.MemoryMB
				capacityUsed.TaskAllocation.DiskMB += *task.Resources.DiskMB
			}
		}
	}

	return nil
}

// MostUtilizedResource calculates the resource that is most-utilized across the cluster.
// This is used to determine the resource that should be prioritized when making scaling
// decisions like determining the least-allocated worker node.
//
// If all resources are completely unutilized, the scaling metric will be set to `None`
// and the daemon will take no actions.
func (c *nomadClient) MostUtilizedResource(alloc *ClusterAllocation) {
	// Determine the resource that is consuming the greatest percentage of its overall cluster
	// capacity.
	max := (Max(alloc.UsedCapacity.CPUPercent, alloc.UsedCapacity.MemoryPercent,
		alloc.UsedCapacity.DiskPercent))

	// Set the compute cluster scaling metric to the most-utilized resource.
	switch max {
	case 0:
		alloc.ScalingMetric = ScalingMetricNone
	case alloc.UsedCapacity.CPUPercent:
		alloc.ScalingMetric = ScalingMetricProcessor
	case alloc.UsedCapacity.DiskPercent:
		alloc.ScalingMetric = ScalingMetricDisk
	case alloc.UsedCapacity.MemoryPercent:
		alloc.ScalingMetric = ScalingMetricMemory
	}
}

// LeastAllocatedNode determines which worker node is consuming the lowest percentage of the
// resource identified as the most-utilized resource across the cluster. Since Nomad follows
// a bin-packing approach, when we need to remove a worker node in response to a scale-in
// activity, we want to identify the least-allocated node and target it for removal.
func (c *nomadClient) LeastAllocatedNode(clusterInfo *ClusterAllocation) (node string) {
	var lowestAllocation float64

	for _, nodeAlloc := range clusterInfo.NodeAllocations {
		switch clusterInfo.ScalingMetric {
		case ScalingMetricProcessor:
			if (lowestAllocation == 0) || (nodeAlloc.UsedCapacity.CPUPercent < lowestAllocation) {
				node = nodeAlloc.NodeID
				lowestAllocation = nodeAlloc.UsedCapacity.CPUPercent
			}
		case ScalingMetricMemory:
			if (lowestAllocation == 0) || (nodeAlloc.UsedCapacity.MemoryPercent < lowestAllocation) {
				node = nodeAlloc.NodeID
				lowestAllocation = nodeAlloc.UsedCapacity.MemoryPercent
			}
		case ScalingMetricDisk:
			if (lowestAllocation == 0) || (nodeAlloc.UsedCapacity.DiskPercent < lowestAllocation) {
				node = nodeAlloc.NodeID
				lowestAllocation = nodeAlloc.UsedCapacity.DiskPercent
			}
		}
	}

	return
}

// DrainNode toggles the drain mode of a worker node. When enabled, no further allocations
// will be assigned and existing allocations will be migrated.
func (c *nomadClient) DrainNode(nodeID string) (err error) {
	// Initiate allocation draining for specified node.
	_, err = c.nomad.Nodes().ToggleDrain(nodeID, true, &nomad.WriteOptions{})
	if err != nil {
		return err
	}

	// Validate node has been placed in drain mode; fail fast if the node
	// failed to enter drain mode.
	resp, _, err := c.nomad.Nodes().Info(nodeID, &nomad.QueryOptions{})
	if (err != nil) || (resp.Drain != true) {
		return err
	}
	fmt.Printf("node %v has been placed in drain mode\n", nodeID)

	// Setup a ticker to poll the node allocations and report when all existing
	// allocations have been migrated to other worker nodes.
	ticker := time.NewTicker(time.Millisecond * 500)
	timeout := time.Tick(time.Minute * 3)

	for {
		select {
		case <-timeout:
			fmt.Printf("timeout %v reached while waiting for existing allocations to be migrated from node %v\n",
				timeout, nodeID)
			return nil
		case <-ticker.C:
			activeAllocations := 0

			// Get allocations assigned to the specified node.
			allocations, _, err := c.nomad.Nodes().Allocations(nodeID, &nomad.QueryOptions{})
			if err != nil {
				return err
			}

			// Iterate over allocations, if any are running or pending, increment the active
			// allocations counter.
			for _, nodeAlloc := range allocations {
				if (nodeAlloc.ClientStatus == structs.JobStatusRunning) || (nodeAlloc.ClientStatus == structs.JobStatusPending) {
					activeAllocations++
				}
			}

			if activeAllocations == 0 {
				fmt.Printf("node %v has no active allocations\n", nodeID)
				return nil
			}

			fmt.Printf("node %v has %v active allocations, pausing and will re-poll allocations\n", nodeID, activeAllocations)
		}
	}
}

// PercentageCapacityRequired accepts a number of cluster allocation parameters
// to then calculate the acceptable percentage of capacity remainining to meet
// the scaling and failure thresholds.
//
// nodeCount:         is the total number of ready worker nodes in the cluster
// allocTotal:        is the allocation totals of each tasks assuming count = 1
// capacityTotal:     is the total cluster allocation capacity
// capacityUsed:      is the total cluster allocation currently in use
// nodeFailureCount:  is the number of acceptable node failures to tollerate
func PercentageCapacityRequired(nodeCount, allocTotal, capacityTotal, capacityUsed, nodeFailureCount int) (capacityRequired float64) {
	nodeAvgAlloc := float64(nodeCount / capacityTotal)
	top := float64((float64(allocTotal)) + (float64(capacityTotal) - (nodeAvgAlloc * float64(nodeFailureCount))))
	capacityRequired = (top / float64(capacityTotal)) * 100
	return capacityRequired
}
