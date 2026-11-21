//go:build darwin && cgo

package main

/*
#include <mach/mach.h>

static kern_return_t cpu_load(host_cpu_load_info_data_t *info) {
	host_t host = mach_host_self();
	mach_msg_type_number_t count = HOST_CPU_LOAD_INFO_COUNT;
	kern_return_t status = host_statistics(host, HOST_CPU_LOAD_INFO, (host_info_t)info, &count);
	mach_port_deallocate(mach_task_self(), host);
	return status;
}
*/
import "C"

import "fmt"

func nativeCPUTicks() (cpuTicks, error) {
	var load C.host_cpu_load_info_data_t
	if status := C.cpu_load(&load); status != C.KERN_SUCCESS {
		return cpuTicks{}, fmt.Errorf("Mach host_statistics returned %d", int(status))
	}
	busy := uint64(load.cpu_ticks[C.CPU_STATE_USER]) + uint64(load.cpu_ticks[C.CPU_STATE_SYSTEM]) + uint64(load.cpu_ticks[C.CPU_STATE_NICE])
	return cpuTicks{busy, uint64(load.cpu_ticks[C.CPU_STATE_IDLE])}, nil
}
