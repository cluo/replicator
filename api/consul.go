package api

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	consul "github.com/hashicorp/consul/api"
)

// ScalingPolicies is a list of ScalingPolicy objects.
type ScalingPolicies []*ScalingPolicy

// ScalingPolicy is a struct which represents an individual jobs scaling policy
// document.
type ScalingPolicy struct {
	// JobName is the name of the Nomad job represented by the Consul Key/Value.
	JobName string

	// Enabled is a boolean falg which dictates whether scaling events for the job
	// should be enforced and is used for testing purposes.
	Enabled bool `json:"enabled"`

	// ScaleOut is the job scaling out policy which will contain the thresholds
	// which control scaling activies.
	ScaleOut *scaleout `json:"scaleout"`

	// ScaleIn is the job scaling in policy which will contain the thresholds
	// which control scaling activies.
	ScaleIn *scalein `json:"scalein"`
}

type scaleout struct {
	CPU int `json:"cpu"`
	MEM int `json:"mem"`
}

type scalein struct {
	CPU int `json:"cpu"`
	MEM int `json:"mem"`
}

// The Client interface is used to provide common method signatures for
// interacting with the Consul API.
type Client interface {
	ListConsulKV(string, string) ([]*ScalingPolicy, error)
}

// The client object is a wrapper to the Consul client provided by the Consul
// API library.
type client struct {
	consul *consul.Client
}

// NewConsulClient is used to construct a new Consul client using the default
// configuration and supporting the ability to specify a Consul API address
// endpoint in the form of address:port.
func NewConsulClient(addr string) (Client, error) {
	// TODO (e.westfall): Add a quick health check call to an API endpoint to
	// validate connectivity or return an error back to the caller.
	config := consul.DefaultConfig()
	config.Address = addr
	c, err := consul.NewClient(config)
	if err != nil {
		// TODO (e.westfall): Raise error here.
		return nil, err
	}

	return &client{consul: c}, nil
}

// ListConsulKV provides a recursed list of Consul KeyValues at the defined
// location and can accept an ACL Token if this is enabled on the Consul cluster
// being used.
func (c *client) ListConsulKV(aclToken, keyLocation string) ([]*ScalingPolicy, error) {

	var entries []*ScalingPolicy

	// Setup the QueryOptions to include the aclToken if this has been set, if not
	// procede with empty QueryOptions struct.
	qop := &consul.QueryOptions{}
	if aclToken != "" {
		qop.Token = aclToken
	}

	// Collect the recursed results from Consul.
	resp, _, err := c.consul.KV().List(keyLocation, qop)
	if err != nil {
		return entries, err
	}

	// Loop the returned list to gather information on each and every job that has
	// a scaling document.
	for _, job := range resp {

		// The results Value is base64 encoded. It is decoded and marshelled into
		// the appropriate struct.
		uEnc := base64.URLEncoding.EncodeToString([]byte(job.Value))
		uDec, _ := base64.URLEncoding.DecodeString(uEnc)
		s := &ScalingPolicy{}
		json.Unmarshal(uDec, s)

		// Trim the Key and its trailing slash to find the job name.
		s.JobName = strings.TrimPrefix(job.Key, keyLocation+"/")

		// Each scaling policy document is then appended to a list to form a full
		// view of all scaling documents available to the cluster.
		entries = append(entries, s)
	}

	return entries, nil
}
