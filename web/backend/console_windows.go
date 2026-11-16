//go:build windows

package main

import "syscall"

// hideLauncherConsoleWindow hides the console window that Windows creates for
// this console-subsystem binary when it is launched from Explorer or the Start
// menu. Building with -H=windowsgui also removes the window, but it silently
// breaks stdout/stderr handling and made the launcher exit early, so the binary
// keeps the console subsystem and hides the window at runtime instead.
func hideLauncherConsoleWindow() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")

	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	const swHide = 0
	_, _, _ = showWindow.Call(hwnd, swHide)
}
