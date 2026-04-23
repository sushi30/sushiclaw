package version

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	GitCommit string
	BuildTime string
	GoVersion string
)

func FormatVersion() string {
	v := Version
	if GitCommit != "" {
		v += fmt.Sprintf(" (git: %s", GitCommit)
		if BuildTime != "" {
			v += fmt.Sprintf(", built: %s", BuildTime)
		}
		goVer := GoVersion
		if goVer == "" {
			goVer = runtime.Version()
		}
		v += fmt.Sprintf(", go: %s)", goVer)
	}
	return v
}
