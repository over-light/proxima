package global

import (
	"fmt"
	"runtime/debug"
)

const (
	// Version has the following structure: vA.B.C[-<label>]
	// A is major version. It is 0 until beta. All alpha testnets are 'v0.n...'. Beta starts at 1
	// B is minor version. Change of the version means breaking change
	// C is subversion. Change usually means non-breaking change
	// <label is arbitrary label>
	Version        = "v0.4.4-testnet"
	bannerTemplate = "starting Proxima node version %s, commit hash: %s, commit time: %s"
)

var (
	CommitHash = "N/A"
	CommitTime = "N/A"
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				CommitHash = setting.Value
			}
			if setting.Key == "vcs.time" {
				CommitTime = setting.Value
			}
		}
	}
}

func BannerString() string {
	return fmt.Sprintf(bannerTemplate, Version, CommitHash, CommitTime)
}
