//go:build !windows

package main

func prepareConsoleForCLI(args []string) {}

func cleanupConsole() {}
