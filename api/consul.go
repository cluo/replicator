package api

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/elsevier-core-engineering/replicator/replicator/structs"
	consul "github.com/hashicorp/consul/api"
)

// The client object is a wrapper to the Consul client provided by the Consul
// API library.
type consulClient struct {
	consul *consul.Client
}

// NewConsulClient is used to construct a new Consul client using the default
// configuration and supporting the ability to specify a Consul API address
// endpoint in the form of address:port.
func NewConsulClient(addr string) (structs.ConsulClient, error) {
	// TODO (e.westfall): Add a quick health check call to an API endpoint to
	// validate connectivity or return an error back to the caller.
	config := consul.DefaultConfig()
	config.Address = addr
	c, err := consul.NewClient(config)
	if err != nil {
		// TODO (e.westfall): Raise error here.
		return nil, err
	}

	return &consulClient{consul: c}, nil
}

// ListConsulKV provides a recursed list of Consul KeyValues at the defined
// location and can accept an ACL Token if this is enabled on the Consul cluster
// being used.
func (c *consulClient) ListConsulKV(config *structs.Config, nomadClient structs.NomadClient) ([]*structs.JobScalingPolicy, error) {
	var entries []*structs.JobScalingPolicy

	// Setup the QueryOptions to include the aclToken if this has been set, if not
	// procede with empty QueryOptions struct.
	qop := &consul.QueryOptions{}
	if config.JobScaling.ConsulToken != "" {
		qop.Token = config.JobScaling.ConsulToken
	}

	// Collect the recursed results from Consul.
	// resp, _, err := c.consul.KV().List(config.JobScaling.ConsulKeyLocation, qop)
	// if err != nil {
	// 	return entries, err
	// }

	kvClient := c.consul.KV()
	resp, _, err := kvClient.List(config.JobScaling.ConsulKeyLocation, qop)
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
		s := &structs.JobScalingPolicy{}
		json.Unmarshal(uDec, s)

		// Trim the Key and its trailing slash to find the job name.
		s.JobName = strings.TrimPrefix(job.Key, config.JobScaling.ConsulKeyLocation+"/")

		// Check to see whether the scaling document is enabled and the job has
		// running task groups before appending to the return.
		if s.Enabled && nomadClient.IsJobRunning(s.JobName) {

			// Each scaling policy document is then appended to a list to form a full
			// view of all scaling documents available to the cluster.
			entries = append(entries, s)
		}
	}

	return entries, nil
}
