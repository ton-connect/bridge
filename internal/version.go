package internal

import "fmt"

const (
	bridgeVersionDefault = "devel"
	gitRevisionDefault   = "devel"
)

var (
	// These variables are here only to show current version. They are set in makefile during build process
	BridgeVersion         = bridgeVersionDefault
	GitRevision           = gitRevisionDefault
	BridgeVersionRevision = func() string {
		version := bridgeVersionDefault
		revision := gitRevisionDefault
		if BridgeVersion != "" {
			version = BridgeVersion
		}
		if GitRevision != "" {
			revision = GitRevision
		}
		return fmt.Sprintf("%s-%s", version, revision)
	}()
)
