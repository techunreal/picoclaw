//go:build !windows

package main

// hideLauncherConsoleWindow is a no-op outside Windows. It keeps the launcher
// entry point identical across builds.
func hideLauncherConsoleWindow() {}
