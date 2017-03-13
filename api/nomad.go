package api

import (
	"fmt"

	nomad "github.com/hashicorp/nomad/api"
)

// The NomadClient interface is used to provide common method signatures for
// interacting with the Nomad API.
type NomadClient interface {
	AllocationCapacity() (*AllocationCapacity, error)
	AssignedAllocation() (*AllocationUsed, error)
	LeaderCheck() bool
}

// The nomadClient object is a wrapper to the Nomad client provided by the
// Nomad API library.
type nomadClient struct {
	nomad *nomad.Client
}

// AllocationCapacity is
type AllocationCapacity struct {
	MemoryMB int
	CPU      int
	DiskMB   int
}

// AllocationUsed is
type AllocationUsed struct {
	MemoryMB int
	CPU      int
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

// AllocationCapacity determines the total cluster allocation capacity.
func (c *nomadClient) AllocationCapacity() (capacity *AllocationCapacity, err error) {

	capacity = &AllocationCapacity{}

	// Get a list of all nodes within the Nomad cluster so that the NodeID can
	// then be interated upon to find node specific resources.
	nodes, _, err := c.nomad.Nodes().List(&nomad.QueryOptions{})
	if err != nil {
		return capacity, err
	}

	// Iterate the NodeID's and call the Nodes.Info endpoint to gather detailed
	// information about the node's resources. If the node is not listed as ready
	// or the node is draining the node is ignored from calculations as we care
	// about the current   available capacity.
	for _, node := range nodes {
		resp, _, err := c.nomad.Nodes().Info(node.ID, &nomad.QueryOptions{})
		if err != nil {
			return capacity, err
		}

		if (resp.Status == "ready") || (resp.Drain != true) {
			capacity.CPU += *resp.Resources.CPU
			capacity.MemoryMB += *resp.Resources.MemoryMB
			capacity.DiskMB += *resp.Resources.DiskMB
		}
	}

	return capacity, nil
}

// AssignedAllocation iterates
func (c *nomadClient) AssignedAllocation() (capacityUsed *AllocationUsed, err error) {

	capacityUsed = &AllocationUsed{}

	// Get a list of all allocations within the Nomad cluster so that the ID can
	// then be interated upon to find allocation specific resources.
	allocs, _, err := c.nomad.Allocations().List(&nomad.QueryOptions{})
	if err != nil {
		return capacityUsed, err
	}

	// Iterate through the allocations list so that we can then call
	// Allocations.Info on each to find what resources are assigned to it.
	for _, alloc := range allocs {
		resp, _, err := c.nomad.Allocations().Info(alloc.ID, &nomad.QueryOptions{})
		if err != nil {
			return capacityUsed, err
		}

		if (resp.ClientStatus == "running") && (resp.DesiredStatus == "run") {
			capacityUsed.CPU += *resp.Resources.CPU
			capacityUsed.MemoryMB += *resp.Resources.MemoryMB
			capacityUsed.DiskMB += *resp.Resources.DiskMB
		}
	}
	return capacityUsed, nil
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
