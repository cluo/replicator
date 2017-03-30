package config

import (
	"fmt"
	"io/ioutil"

	"github.com/elsevier-core-engineering/replicator/api"
	"github.com/elsevier-core-engineering/replicator/logging"
	"github.com/elsevier-core-engineering/replicator/replicator/structs"

	conf "github.com/hashicorp/consul-template/config"
	"github.com/hashicorp/hcl"
	"github.com/mitchellh/mapstructure"
)

// DefaultConfig returns a default configuration struct with sane defaults.
func DefaultConfig() *structs.Config {

	return &structs.Config{
		Consul:   "localhost:8500",
		Nomad:    "http://localhost:4646",
		LogLevel: "INFO",
		Enforce:  true,

		ClusterScaling: &structs.ClusterScaling{
			MaxSize:            10,
			MinSize:            5,
			CoolDown:           300,
			NodeFaultTolerance: 1,
		},

		JobScaling: &structs.JobScaling{
			ConsulKeyLocation: "replicator/config/jobs",
		},

		Telemetry: &structs.Telemetry{},
	}
}

// ParseConfig reads the configuration file at the given path and returns a new
// Config struct with the data populated. The returned data is a merge of the
// user specified parameters and defaults with the user provided params always
// taking precident.
func ParseConfig(path string) (*structs.Config, error) {

	// Read the configuration file
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config at %q: %s", path, err)
	}

	var shadow interface{}
	if err := hcl.Decode(&shadow, string(contents)); err != nil {
		return nil, fmt.Errorf("error decoding config at %q: %s", path, err)
	}

	parsed, ok := shadow.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("error converting config at %q", path)
	}
	flattenKeys(parsed, []string{"cluster_scaling"})
	flattenKeys(parsed, []string{"job_scaling"})
	flattenKeys(parsed, []string{"telemetry"})

	c := new(structs.Config)

	// Using mapstrcuture we populate the user defined population feilds.
	metadata := new(mapstructure.Metadata)
	decoder, _ := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			conf.StringToWaitDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			mapstructure.StringToTimeDurationHookFunc(),
		),
		ErrorUnused: true,
		Metadata:    metadata,
		Result:      c,
	})
	if err := decoder.Decode(parsed); err != nil {
		return nil, err
	}

	if c.SetKeys == nil {
		c.SetKeys = make(map[string]struct{})
	}
	for _, key := range metadata.Keys {
		if _, ok := c.SetKeys[key]; !ok {
			c.SetKeys[key] = struct{}{}
		}
	}

	// Merge the user provided configuration parameters into the default
	// configuration to result in a full configuration set.
	d := DefaultConfig()
	d.Merge(c)
	c = d

	// Instantiate a new Consul client.
	if consulClient, err := api.NewConsulClient(c.Consul); err == nil {
		c.ConsulClient = consulClient
	} else {
		logging.Error("failed to establish a new consul client: %v", err)
	}

	// Instantiate a new Nomad client.
	if nomadClient, err := api.NewNomadClient(c.Nomad); err == nil {
		c.NomadClient = nomadClient
	} else {
		logging.Error("failed to establish a new nomad client: %v", err)
	}

	return c, nil
}

// flattenKeys is a function that takes a map[string]interface{} and recursively
// flattens any keys that are a []map[string]interface{} where the key is in the
// given list of keys.
func flattenKeys(m map[string]interface{}, keys []string) {
	keyMap := make(map[string]struct{})
	for _, key := range keys {
		keyMap[key] = struct{}{}
	}

	var flatten func(map[string]interface{})
	flatten = func(m map[string]interface{}) {
		for k, v := range m {
			if _, ok := keyMap[k]; !ok {
				continue
			}

			switch typed := v.(type) {
			case []map[string]interface{}:
				if len(typed) > 0 {
					last := typed[len(typed)-1]
					flatten(last)
					m[k] = last
				} else {
					m[k] = nil
				}
			case map[string]interface{}:
				flatten(typed)
				m[k] = typed
			default:
				m[k] = v
			}
		}
	}
	flatten(m)
}
