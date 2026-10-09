package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const attachParentProcess = ^uintptr(0)

var attachConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")

func prepareConsole() bool {
	out, errOut := usable(windows.STD_OUTPUT_HANDLE), usable(windows.STD_ERROR_HANDLE)
	if out && errOut {
		return true
	}
	if r, _, _ := attachConsole.Call(attachParentProcess); r == 0 {
		return errOut
	}
	f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		return errOut
	}
	if !out {
		os.Stdout = f
	}
	if !errOut {
		os.Stderr = f
	}
	return true
}

func usable(std uint32) bool {
	h, _ := windows.GetStdHandle(std)
	return h != 0 && h != windows.InvalidHandle
}

func defaultLogFile() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "flavor", "flavord.log")
}
