package main

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

func nativeCPUTicks() (cpuTicks, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTicks{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return cpuTicks{}, errors.New("missing CPU line in /proc/stat")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 9 || fields[0] != "cpu" {
		return cpuTicks{}, errors.New("invalid CPU line in /proc/stat")
	}
	var ticks [8]uint64
	for i := range ticks {
		ticks[i], err = strconv.ParseUint(fields[i+1], 10, 64)
		if err != nil {
			return cpuTicks{}, err
		}
	}
	// Guest ticks are already included in user and nice.
	return cpuTicks{ticks[0] + ticks[1] + ticks[2] + ticks[5] + ticks[6] + ticks[7], ticks[3] + ticks[4]}, nil
}
