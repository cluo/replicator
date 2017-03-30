package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/elsevier-core-engineering/replicator/api"
	"github.com/elsevier-core-engineering/replicator/replicator/structs"
	"github.com/hashicorp/consul-template/test"
)

func TestParseConfig_correctDefaulValues(t *testing.T) {
	// TODO (e.westfall): Can we call ParseConfig in here to pickup the client
	// instantiation?
	config := DefaultConfig()
	consulClient, _ := api.NewConsulClient("localhost:8500")
	config.ConsulClient = consulClient

	expected := &structs.Config{
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
	}

	if !reflect.DeepEqual(config, expected) {
		t.Fatalf("expected \n%#v\n\n, got \n\n%#v\n\n", expected, config)
	}
}

// func TestParseConfig_correctNestedPartialOverride(t *testing.T) {
// 	configFile := test.CreateTempfile([]byte(`
//     consul  = "consul.tiorap.systems:8500"
// 		nomad   = "nomad.tiorap.systems:4646"
//
//     cluster_scaling {
//       max_size = 15
//     }
//   `), t)
// 	defer test.DeleteTempfile(configFile, t)
//
// 	c, err := ParseConfig(configFile.Name())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
//
// 	consulClient, _ := api.NewConsulClient("consul.tiorap.systems:8500")
//
// 	expected := &structs.Config{
// 		Consul:   "consul.tiorap.systems:8500",
// 		Nomad:    "nomad.tiorap.systems:4646",
// 		LogLevel: "INFO",
// 		Enforce:  true,
//
// 		ClusterScaling: &structs.ClusterScaling{
// 			MaxSize:            15,
// 			MinSize:            5,
// 			CoolDown:           300,
// 			NodeFaultTolerance: 1,
// 		},
//
// 		JobScaling: &structs.JobScaling{
// 			ConsulKeyLocation: "replicator/config/jobs",
// 		},
//
// 		Telemetry:    &structs.Telemetry{},
// 		ConsulClient: consulClient,
// 	}
// 	if !reflect.DeepEqual(c, expected) {
// 		t.Fatalf("expected \n%#v\n\n, got \n\n%#v\n\n", expected, c)
// 	}
// }

// func TestParseConfig_correctFullOverride(t *testing.T) {
// 	configFile := test.CreateTempfile([]byte(`
//     consul    = "consul.tiorap.systems:8500"
//     nomad     = "nomad.tiorap.systems:4646"
//     log_level = "DEBUG"
//     enforce   = false
//
//     cluster_scaling {
//       max_size             = 1000
//       min_size             = 100
// 			cool_down            = 100
// 			node_fault_tolerance = 50
//     }
//
//     job_scaling {
//       consul_key_location = "tiorap/replicator/config"
//       consul_token        = "supersecrettokenthingy"
//     }
//
//     telemetry {
//       statsd_address = "statsd.tiorap.systems:8125"
//     }
//
//   `), t)
// 	defer test.DeleteTempfile(configFile, t)
//
// 	c, err := ParseConfig(configFile.Name())
// 	if err != nil {
// 		t.Fatal(err)
// 	}
//
// 	expected := &structs.Config{
// 		Consul:   "consul.tiorap.systems:8500",
// 		Nomad:    "nomad.tiorap.systems:4646",
// 		LogLevel: "DEBUG",
// 		Enforce:  false,
//
// 		ClusterScaling: &structs.ClusterScaling{
// 			MaxSize:            1000,
// 			MinSize:            100,
// 			CoolDown:           100,
// 			NodeFaultTolerance: 50,
// 		},
//
// 		JobScaling: &structs.JobScaling{
// 			ConsulKeyLocation: "tiorap/replicator/config",
// 			ConsulToken:       "supersecrettokenthingy",
// 		},
//
// 		Telemetry: &structs.Telemetry{
// 			StatsdAddress: "statsd.tiorap.systems:8125",
// 		},
// 	}
//
// 	if !reflect.DeepEqual(c, expected) {
// 		t.Fatalf("expected \n%#v\n\n, got \n\n%#v\n\n", expected, c)
// 	}
// }

func TestParseConfig_hclSyntaxIssue(t *testing.T) {
	configFile := test.CreateTempfile([]byte(`
    consul  = "consul.tiorap.systems:8500"
    nomad   = "nomad.tiorap.systems:4646"

    cluster_scaling {
      max_size = 15

  `), t)
	defer test.DeleteTempfile(configFile, t)

	expected := "error decoding config at"

	_, err := ParseConfig(configFile.Name())

	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected %q to include %q", err.Error(), expected)
	}
}
