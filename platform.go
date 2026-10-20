package main

import "runtime"

func platformText(mac, win string) string {
	if runtime.GOOS == "windows" {
		return win
	}
	return mac
}
func platformName() string { return platformText("macOS", "Windows") }
