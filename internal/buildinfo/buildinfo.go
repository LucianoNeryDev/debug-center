// Package buildinfo holds values stamped into the binaries at build time with
// -ldflags "-X". Baking the tailnet settings into the agent lets a technician
// hand a single executable to a customer machine and install it with no flags.
package buildinfo

import "strings"

var (
	// Version is the release version.
	Version = "dev"
	// ControlURL is the default coordination server (Headscale) URL. Empty
	// means the Tailscale default control plane.
	ControlURL = ""
	// AllowPeers is a comma-separated list of tailnet identities (login names
	// or tag:… values) allowed to control the agent.
	AllowPeers = ""
	// AgentTags is a comma-separated list of tags the agent advertises.
	AgentTags = ""
	// AuthKey is an optional pre-auth key. Prefer passing it at install time;
	// anything baked here can be extracted from the binary.
	AuthKey = ""
)

// List splits a comma-separated build value into trimmed, non-empty items.
func List(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
