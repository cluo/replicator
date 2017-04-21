package version

import "fmt"

// The main version number that is being run at the moment.
const version = "0.0.1"

// A pre-release marker for the version. If this is "" (empty string)
// then it means that it is a final release. Otherwise, this is a pre-release
// such as "dev" (in development), "beta", "rc1", etc.
const versionPrerelease = "dev"

// Get returns a human readable version of replicator.
func Get() string {

	if versionPrerelease != "" {
		return fmt.Sprintf("%s-%s", version, versionPrerelease)
	}

	return version
}
