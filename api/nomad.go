package api

import (
	"fmt"

	nomad "github.com/hashicorp/nomad/api"
)

// The NomadClient interface is used to provide common method signatures for
// interacting with the Nomad API.
type NomadClient interface {
	ClusterAllocationCapacity(*ClusterAllocation) error
	ClusterAssignedAllocation(*ClusterAllocation) error
	TaskAllocationTotals(*ClusterAllocation) error
	LeaderCheck() bool
}

// The nomadClient object is a wrapper to the Nomad client provided by the
// Nomad API library.
type nomadClient struct {
	nomad *nomad.Client
}

// ClusterAllocation is the main struct used for compiling information regarding
// the cluster state which allows us to make cluster scaling decisions.
type ClusterAllocation struct {
	// NodeCount is the count of worker nodes in a ready and non-draining state in
	// the cluster.
	NodeCount int

	// ClusterTotalAllocationCapacity is the total allocation which the cluster
	// can support.
	ClusterTotalAllocationCapacity totalAllocationCapacity

	// ClusterUsedAllocationCapacity is the currently used cluster allocation
	// across the cluster.
	ClusterUsedAllocationCapacity usedAllocationCapacity

	// TaskAllocation is the allocation total of all running jobs on the cluster
	// assuming the count = 1. This is used in order to ensure the cluster has
	// enough free capacity to scale each task by 1 if an increase in capacity is
	// required.
	TaskAllocation taskAllocation
}

type totalAllocationCapacity struct {
	MemoryMB int
	CPUMHz   int
	DiskMB   int
}

type usedAllocationCapacity struct {
	MemoryMB int
	CPUMHz   int
	DiskMB   int
}

type taskAllocation struct {
	MemoryMB int
	CPUMHz   int
	DiskMB   int
}

// NewNomadClient is used to construct a new Nomad client using the default
// configuration and supporting the ability to specify a Nomad API address
// endpoint in the form of address:port.
func NewNomadClient(addr string) (NomadClient, error) {
	config := nomad.DefaultConfig()
	config.Address = addr
	c, err := nomad.NewClient(config)
	if err != nil {
		return nil, err
	}

	return &nomadClient{nomad: c}, nil
}

// ClusterAllocationCapacity determines the total cluster allocation capacity as
// well as the total number of available worker nodes.
func (c *nomadClient) ClusterAllocationCapacity(capacity *ClusterAllocation) (err error) {

	// Get a list of all nodes within the Nomad cluster so that the NodeID can
	// then be interated upon to find node specific resources.
	nodes, _, err := c.nomad.Nodes().List(&nomad.QueryOptions{})
	if err != nil {
		return err
	}

	// Iterate the NodeID's and call the Nodes.Info endpoint to gather detailed
	// information about the node's resources. If the node is not listed as ready
	// or the node is draining the node is ignored from calculations as we care
	// about the current   available capacity.
	for _, node := range nodes {
		resp, _, err := c.nomad.Nodes().Info(node.ID, &nomad.QueryOptions{})
		if err != nil {
			return err
		}

		if (resp.Status == "ready") || (resp.Drain != true) {
			capacity.NodeCount++
			capacity.ClusterTotalAllocationCapacity.CPUMHz += resp.Resources.CPU
			capacity.ClusterTotalAllocationCapacity.MemoryMB += resp.Resources.MemoryMB
			capacity.ClusterTotalAllocationCapacity.DiskMB += resp.Resources.DiskMB
		}
	}

	return nil
}

// ClusterAssignedAllocation iterates the current cluster allocations to provide
// resource totals of what is currently running.
func (c *nomadClient) ClusterAssignedAllocation(capacityUsed *ClusterAllocation) (err error) {

	// Get a list of all allocations within the Nomad cluster so that the ID can
	// then be interated upon to find allocation specific resources.
	allocs, _, err := c.nomad.Allocations().List(&nomad.QueryOptions{})
	if err != nil {
		return err
	}

	// Iterate through the allocations list so that we can then call
	// Allocations.Info on each to find what resources are assigned to it.
	for _, alloc := range allocs {
		resp, _, err := c.nomad.Allocations().Info(alloc.ID, &nomad.QueryOptions{})
		if err != nil {
			return err
		}

		if (resp.ClientStatus == "running") && (resp.DesiredStatus == "run") {
			capacityUsed.ClusterUsedAllocationCapacity.CPUMHz += resp.Resources.CPU
			capacityUsed.ClusterUsedAllocationCapacity.MemoryMB += resp.Resources.MemoryMB
			capacityUsed.ClusterUsedAllocationCapacity.DiskMB += resp.Resources.DiskMB
		}
	}
	return nil
}

// LeaderCheck determines if the local node has cluster leadership.
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
		attributes := self["member"]

		if findIP(leader) == attributes["Addr"].(string) {
			haveLeadership = true
		}
	}

	return haveLeadership
}

// TaskAllocationTotals iterates through the running jobs on the cluster to
// determine the allocations required to scale each job task by a count of 1.
// This is used to ensure the cluster have enough capacity for scaling events
// as well as node failure events.
func (c *nomadClient) TaskAllocationTotals(capacityUsed *ClusterAllocation) error {

	// Get a list of all the jobs on the cluster currently so that we can iterate
	// the job.ID to find the total task resources assigned per job.
	jobs, _, err := c.nomad.Jobs().List(&nomad.QueryOptions{})
	if err != nil {
		return err
	}

	// Iterate through the jobs list using the job.ID to call more detailed info
	// about the job in order to discover
	for _, job := range jobs {
		resp, _, err := c.nomad.Jobs().Info(job.ID, &nomad.QueryOptions{})
		if err != nil {
			return err
		}

		// A job can contain multiple taskgroups which can intern contain multiple
		// tasks; therefore we must iterate this fully. Nomad will only return jobs
		// that are running, so we do not need to check the status.
		for _, taskG := range resp.TaskGroups {
			for _, task := range taskG.Tasks {
				capacityUsed.TaskAllocation.CPUMHz += task.Resources.CPU
				capacityUsed.TaskAllocation.MemoryMB += task.Resources.MemoryMB
				capacityUsed.TaskAllocation.DiskMB += task.Resources.DiskMB
			}
		}
	}
	return nil
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
