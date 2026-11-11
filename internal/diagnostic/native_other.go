//go:build !windows || !amd64

package diagnostic

// Tests on Ubuntu exercise injected synthetic Probes. Native probing is
// deliberately unavailable outside the approved Windows x64 target.
func NativeProbes()Probes{return Probes{}}
