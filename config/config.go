package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/elsevier-core-engineering/replicator/api"
	"github.com/elsevier-core-engineering/replicator/logging"
	"github.com/elsevier-core-engineering/replicator/replicator/structs"

	conf "github.com/hashicorp/consul-template/config"
	"github.com/hashicorp/hcl"
	"github.com/mitchellh/mapstructure"
)

// Define default local addresses for Consul and Nomad
const (
	LocalConsulAddress = "localhost:8500"
	LocalNomadAddress  = "http://localhost:4646"
)

// DefaultConfig returns a default configuration struct with sane defaults.
func DefaultConfig() *structs.Config {
	// var consulClient structs.ConsulClient
	// var nomadClient structs.NomadClient

	// Instantiate a new Consul client.
	consulClient, err := api.NewConsulClient(LocalConsulAddress)
	if err != nil {
		logging.Error("failed to obtain consul connection: %v", err)
	}

	// Instantiate a new Nomad client.
	nomadClient, err := api.NewNomadClient(LocalNomadAddress)
	if err != nil {
		logging.Error("failed to obtain nomad connection: %v", err)
	}

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

		Telemetry:    &structs.Telemetry{},
		ConsulClient: consulClient,
		NomadClient:  nomadClient,
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

// FromPath iterates and merges all configuration files in a given directory
// returning the resulting merged config based on lexical ordering for all
// files found. If the passed object is a file this is read and merged into the
// default config.
func FromPath(path string) (*structs.Config, error) {

	// Ensure the given filepath exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("the config file/folder %s is missing", path)
	}

	// Check if a file was given or a path to a directory
	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("unable to stat the config file %s", err)
	}

	// Recursively parse directories, single load files
	if stat.Mode().IsDir() {

		_, err := ioutil.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("error listing config directory %s", err)
		}

		// Create a blank config to merge off of
		config := DefaultConfig()

		err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Do nothing for directories
			if info.IsDir() {
				return nil
			}

			// Parse and merge the config
			newConfig, err := ParseConfig(path)

			if err != nil {
				return err
			}
			newConfig.Merge(config)
			config = newConfig

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("config: walk error: %s", err)
		}

		return config, nil
	} else if stat.Mode().IsRegular() {
		return ParseConfig(path)
	}

	return nil, fmt.Errorf("unknown config filetype %q", stat.Mode().String())
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
