package monitor

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// procTaskInfo matches struct proc_taskinfo in macOS sys/proc_info.h.
// Reuse one proc_pidinfo binding, read CPU and RSS together, and verify
// the returned byte count. An inaccessible process is not an idle process.
type procTaskInfo struct {
	Virtual, Resident, User, System, ThreadsUser, ThreadsSystem                 uint64
	Policy, Faults, Pageins, CowFaults, MessagesSent, MessagesReceived          int32
	SyscallsMach, SyscallsUnix, ContextSwitches, ThreadCount, Running, Priority int32
}

type taskInfo struct {
	seconds float64
	memory  uint64
	threads int32
	started int64
}

var taskAPI struct {
	once    sync.Once
	err     error
	pidInfo func(int32, int32, uint64, unsafe.Pointer, int32) int32
	scale   float64
}

func readTask(pid int32) (taskInfo, error) {
	taskAPI.once.Do(func() {
		// These system-library handles live for the lifetime of the app. Closing
		// them would invalidate the registered functions while a reader is active.
		lib, err := purego.Dlopen("/usr/lib/libproc.dylib", purego.RTLD_LAZY|purego.RTLD_LOCAL)
		if err != nil {
			taskAPI.err = err
			return
		}
		pidSym, err := purego.Dlsym(lib, "proc_pidinfo")
		if err != nil {
			taskAPI.err = err
			return
		}
		purego.RegisterFunc(&taskAPI.pidInfo, pidSym)
		system, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_LOCAL)
		if err != nil {
			taskAPI.err = err
			return
		}
		timeSym, err := purego.Dlsym(system, "mach_timebase_info")
		if err != nil {
			taskAPI.err = err
			return
		}
		var timebase func(unsafe.Pointer) int32
		purego.RegisterFunc(&timebase, timeSym)
		var base struct{ Numer, Denom uint32 }
		if timebase(unsafe.Pointer(&base)) != 0 || base.Denom == 0 {
			taskAPI.err = fmt.Errorf("无法读取 CPU 时间单位")
			return
		}
		taskAPI.scale = float64(base.Numer) / float64(base.Denom) / 1e9
	})
	if taskAPI.err != nil {
		return taskInfo{}, taskAPI.err
	}
	var info procTaskInfo
	const procPIDTaskInfo = 4
	size := int32(unsafe.Sizeof(info))
	if n := taskAPI.pidInfo(pid, procPIDTaskInfo, 0, unsafe.Pointer(&info), size); n != size {
		return taskInfo{}, fmt.Errorf("进程 %d 已退出或不允许读取资源信息", pid)
	}
	return taskInfo{seconds: (float64(info.User) + float64(info.System)) * taskAPI.scale, memory: info.Resident, threads: info.ThreadCount}, nil
}
