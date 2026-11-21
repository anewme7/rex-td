//go:build (!darwin && !linux) || (darwin && !cgo)

package main

import "errors"

func nativeCPUTicks() (cpuTicks, error) {
	return cpuTicks{}, errors.New("native CPU sampling unavailable in this build")
}
