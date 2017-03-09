package replicator

import (
	"fmt"
	"io/ioutil"

	conf "github.com/hashicorp/consul-template/config"
	"github.com/hashicorp/hcl"
	"github.com/mitchellh/mapstructure"
)

// DefaultConfig returns a default configuration struct with sane defaults.
func DefaultConfig() *Config {

	return &Config{
		Consul:   "localhost:8500",
		Nomad:    "localhost:4646",
		LogLevel: "INFO",
		Enforce:  true,

		ClusterScaling: &ClusterScaling{
			MaxSize: 10,
			MinSize: 5,
		},

		JobScaling: &JobScaling{
			ConsulKeyLocation: "replicator/config/jobs",
		},

		Telemetry: &Telemetry{},
	}
}

// ParseConfig reads the configuration file at the given path and returns a new
// Config struct with the data populated. The returned data is a merge of the
// user specified parameters and defaults with the user provided params always
// taking precident.
func ParseConfig(path string) (*Config, error) {

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

	c := new(Config)

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

	if c.setKeys == nil {
		c.setKeys = make(map[string]struct{})
	}
	for _, key := range metadata.Keys {
		if _, ok := c.setKeys[key]; !ok {
			c.setKeys[key] = struct{}{}
		}
	}

	// Merge the user provided configuration parameters into the default
	// configuration to result in a full configuration set.
	d := DefaultConfig()
	d.Merge(c)
	c = d

	return c, nil
}

// Merge takes the user override parameters and merges these into the default
// config parameters. User overrides will always take priority.
func (c *Config) Merge(o *Config) {
	if o.WasSet("consul") {
		c.Consul = o.Consul
	}
	if o.WasSet("nomad") {
		c.Nomad = o.Nomad
	}
	if o.WasSet("log_level") {
		c.LogLevel = o.LogLevel
	}
	if o.WasSet("enforce") {
		c.Enforce = o.Enforce
	}
	if o.WasSet("cluster_scaling") {
		if o.WasSet("cluster_scaling.max_size") {
			c.ClusterScaling.MaxSize = o.ClusterScaling.MaxSize
		}
		if o.WasSet("cluster_scaling.min_size") {
			c.ClusterScaling.MinSize = o.ClusterScaling.MinSize
		}
	}
	if o.WasSet("job_scaling") {
		if o.WasSet("job_scaling.consul_token") {
			c.JobScaling.ConsulToken = o.JobScaling.ConsulToken
		}
		if o.WasSet("job_scaling.consul_key_location") {
			c.JobScaling.ConsulKeyLocation = o.JobScaling.ConsulKeyLocation
		}
	}
	if o.WasSet("telemetry") {
		if o.WasSet("telemetry.statsd_address") {
			c.Telemetry.StatsdAddress = o.Telemetry.StatsdAddress
		}
	}
}

// WasSet determines if the given key was set by the user or uses the default
// values.
func (c *Config) WasSet(key string) bool {
	if _, ok := c.setKeys[key]; ok {
		return true
	}
	return false
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
