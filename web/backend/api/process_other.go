//go:build !windows

package api

// inspectGatewayProcessWindows is only used on Windows.
func inspectGatewayProcessWindows(pid int) (bool, bool) {
	return false, false
}
