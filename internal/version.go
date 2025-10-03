package internal

import "fmt"

var (
	// These variables are here only to show current version. They are set in makefile during build process
	BridgeVersion         = "devel"
	GitRevision           = "devel"
	BridgeVersionRevision = fmt.Sprintf("%s-%s", BridgeVersion, GitRevision)
)
