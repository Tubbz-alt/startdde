package swapsched

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
)

const (
	memoryCtrl       = "memory"
	freezerCtrl      = "freezer"
	SystemCGroupRoot = "/sys/fs/cgroup"
)

const KB = 1024
const MB = 1024 * KB

func joinCGPath(args ...string) string {
	return path.Join(SystemCGroupRoot, path.Join(args...))
}

func cgCreate(ctrl string, path string) error {
	return os.MkdirAll(joinCGPath(ctrl, path), 0700)
}
func cgDelete(ctrl string, path string) error {
	return os.Remove(joinCGPath(ctrl, path))
}

// getSystemMemoryInfo 返回 系统可用内存, 系统已用Swap
func getSystemMemoryInfo() (uint64, uint64) {
	MemAvailable, SwapTotal, SwapFree := "MemAvailable", "SwapTotal", "SwapFree"
	vs := ParseMemoryStatKB("/proc/meminfo", MemAvailable, SwapTotal, SwapFree)
	return vs[MemAvailable], vs[SwapTotal] - vs[SwapFree]
}

func getProcessesSwap(pids ...int) uint64 {
	VmSwap := "VmSwap"
	ret := uint64(0)
	for _, pid := range pids {
		ret += ParseMemoryStatKB(fmt.Sprintf("/proc/%d/status", pid), VmSwap)[VmSwap]
	}
	return ret
}

func toUint64(v []byte, hasErr error) uint64 {
	if hasErr != nil {
		return 0
	}
	ret, _ := strconv.ParseUint(strings.TrimSpace(string(v)), 10, 64)
	return ret
}

func toLines(v []byte, hasErr error) []string {
	if hasErr != nil {
		return nil
	}
	var ret []string
	for _, line := range strings.Split(string(v), "\n") {
		if line != "" {
			ret = append(ret, line)
		}
	}
	return ret
}

func freezeUIApps(cgroup string) error {
	return writeCGroupFile(freezerCtrl, cgroup, "freezer.state", "FROZEN")
}
func thawUIApps(cgroup string) error {
	return writeCGroupFile(freezerCtrl, cgroup, "freezer.state", "THAWED")
}

func readCGroupFile(ctrl string, name string, key string) ([]byte, error) {
	return ioutil.ReadFile(joinCGPath(ctrl, name, key))
}

func writeCGroupFile(ctrl string, name string, key string, value interface{}) error {
	fpath := joinCGPath(ctrl, name, key)
	return ioutil.WriteFile(fpath, []byte(fmt.Sprintf("%v", value)), 0777)
}

func getCGroupPIDs(ctrl string, name string) []int {
	var pids []int
	for _, line := range toLines(readCGroupFile(ctrl, name, "cgroup.procs")) {
		pid, _ := strconv.ParseInt(line, 10, 32)
		if pid != 0 {
			pids = append(pids, int(pid))
		}
	}
	return pids
}

func setLimitRSS(cgroup string, v uint64) error {
	return writeCGroupFile(memoryCtrl, cgroup, "memory.soft_limit_in_bytes", v)
}

func setHardLimit(cgroup string, v uint64) error {
	return writeCGroupFile(memoryCtrl, cgroup, "memory.limit_in_bytes", v)
}

// ParseMemoryStat parse the /sys/fs/cgroup/memory/$appGroupName/memory.stat
func ParseMemoryStat(appGroupName string, keys ...string) map[string]uint64 {
	ret := make(map[string]uint64)
	for _, line := range toLines(readCGroupFile(memoryCtrl, appGroupName, "memory.stat")) {
		for _, key := range keys {
			if strings.HasPrefix(line, key) {
				v, _ := strconv.ParseUint(line[len(key):], 10, 64)
				ret[key] = v
				if len(ret) >= len(keys) {
					return ret
				}
			}
		}
	}
	return ret
}

// ParseMemoryStatKB parse fields with KB suffix in /proc/self/status, /proc/meminfo
func ParseMemoryStatKB(filePath string, keys ...string) map[string]uint64 {
	ret := make(map[string]uint64)
	for _, line := range toLines(ioutil.ReadFile(filePath)) {
		fields := strings.Split(line, ":")
		if len(fields) != 2 {
			continue
		}
		key := strings.TrimSpace(fields[0])
		for _, ikey := range keys {
			if key == ikey {
				value := strings.TrimSpace(fields[1])
				value = strings.Replace(value, " kB", "", -1)
				v, _ := strconv.ParseUint(value, 10, 64)
				ret[key] = v * KB
				if len(ret) >= len(keys) {
					return ret
				}
			}
		}
	}
	return ret
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func mapKeys(m map[string]uint64) []string {
	var ret []string
	for k := range m {
		ret = append(ret, k)
	}
	return ret
}
