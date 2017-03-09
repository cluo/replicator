package replicator

// Config is the main configuration struct used to configure the replicator
// application.
type Config struct {
	// Consul is the location of the Consul instance or cluster endpoint to query
	// (may be an IP address or FQDN) with port.
	Consul string `mapstructure:"consul"`

	// Nomad is the location of the Nomad instance or cluster endpoint to query
	// (may be an IP address or FQDN) with port.
	Nomad string `mapstructure:"nomad"`

	// LogLevel is the level at which the application should log from.
	LogLevel string `mapstructure:"log_level"`

	// Enforce is the boolean falg which dicates whether or not scaling events are
	// actioned, or whether the application runs in report only mode.
	Enforce bool `mapstructure:"enforce"`

	// ClusterScaling is the configuration struct that controls the basic Nomad
	// worker node scaling.
	ClusterScaling *ClusterScaling `mapstructure:"cluster_scaling"`

	// JobScaling is the configuration struct that controls the basic Nomad
	// job scaling.
	JobScaling *JobScaling `mapstructure:"job_scaling"`

	// Telemetry is the configuration struct that controls the telemetry settings.
	Telemetry *Telemetry `mapstructure:"telemetry"`

	// setKeys is the list of config keys that were overridden by the user.
	setKeys map[string]struct{}
}

// ClusterScaling is the configuration struct for the Nomad worker node scaling
// activites.
type ClusterScaling struct {
	// MaxSize in the maximum number of instances the nomad node worker count is
	// allowed to reach. This stops runaway increases in size due to misbehaviour
	// but should be set high enough to accomodate usual workload peaks.
	MaxSize float64 `mapstructure:"max_size"`

	// MinSize is the minimum number of instances that should be present within
	// the nomad node worker pool.
	MinSize float64 `mapstructure:"min_size"`
}

// JobScaling is the configuration struct for the Nomad job scaling activities.
type JobScaling struct {
	// ConsulToken is the Consul ACL token used to access KeyValues from a
	// secure Consul installation.
	ConsulToken string `mapstructure:"consul_token"`

	// ConsulKeyLocation is the Consul key location where scaling policies are
	// defined.
	ConsulKeyLocation string `mapstructure:"consul_key_location"`
}

// Telemetry is the struct that control the telemetry configuration. If a value
// is present then telemetry is enabled. Currently statsd is only supported for
// sending telemetry.
type Telemetry struct {
	// StatsdAddress specifies the address of a statsd server to forward metrics
	// to and should include the port.
	StatsdAddress string `mapstructure:"statsd_address"`
}
