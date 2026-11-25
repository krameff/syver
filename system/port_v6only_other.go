//go:build !linux

package system

// wildcardV6Only has no reader outside Linux: every socket on [::] is
// v6onlyUnknown, so no dual-stack listener is counted for tcp:/udp:.
func wildcardV6Only(_ uint8) (map[uint32]v6onlyState, error) {
	return nil, nil
}
