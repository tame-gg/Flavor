//go:build !windows

package main

func prepareConsole() bool { return true }

func defaultLogFile() string { return "" }
