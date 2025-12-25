package version

// Variables set via ldflags at build time
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
	BuildType = "Dev"
)

type BuildInfo struct {
	Version   string
	GitCommit string
	BuildDate string
	BuildType string
}

func Get() BuildInfo {
	return BuildInfo{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		BuildType: BuildType,
	}
}
