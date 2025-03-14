package version

// Version is the current version of Turkis
// This will be overridden during build when using ldflags
var Version = "v0.1.12"

// GetVersion returns the current version string
func GetVersion() string {
	return Version
}
