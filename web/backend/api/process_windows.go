//go:build windows

package api

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

type gatewayPidCacheEntry struct {
	match   bool
	checked time.Time
}

var (
	gatewayPidCacheMu sync.Mutex
	gatewayPidCache   = map[int]gatewayPidCacheEntry{}
)

// inspectGatewayProcessWindows checks the process image name without spawning a
// helper process. PowerShell/tasklist calls here used to flash a console window
// every few seconds because the status endpoint polls the gateway PID.
func inspectGatewayProcessWindows(pid int) (bool, bool) {
	if pid <= 0 {
		return false, true
	}

	gatewayPidCacheMu.Lock()
	if entry, ok := gatewayPidCache[pid]; ok && time.Since(entry.checked) < 2*time.Minute {
		gatewayPidCacheMu.Unlock()
		return entry.match, true
	}
	gatewayPidCacheMu.Unlock()

	match, inspected := gatewayImageNameMatches(pid)
	if inspected {
		gatewayPidCacheMu.Lock()
		gatewayPidCache[pid] = gatewayPidCacheEntry{match: match, checked: time.Now()}
		// Keep the cache small; entries are only useful for recent PIDs.
		if len(gatewayPidCache) > 32 {
			for key, entry := range gatewayPidCache {
				if time.Since(entry.checked) > 10*time.Minute {
					delete(gatewayPidCache, key)
				}
			}
		}
		gatewayPidCacheMu.Unlock()
	}
	return match, inspected
}

func gatewayImageNameMatches(pid int) (bool, bool) {
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(pid),
	)
	if err != nil {
		return false, false
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return false, false
	}
	image := windows.UTF16ToString(buf[:size])
	name := strings.ToLower(filepath.Base(image))
	return name == "picoclaw.exe" || name == "picoclaw", true
}
