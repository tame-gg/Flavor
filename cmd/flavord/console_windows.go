package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const attachParentProcess = ^uintptr(0)

var attachConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")

func prepareConsole() bool {
	if h, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE); h != 0 && h != windows.InvalidHandle {
		return true
	}
	if r, _, _ := attachConsole.Call(attachParentProcess); r == 0 {
		return false
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
	return true
}

func defaultLogFile() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "flavor", "flavord.log")
}
