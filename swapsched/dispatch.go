package swapsched

import (
	"fmt"
	"os"
	"pkg.deepin.io/lib/log"
	"sync"
	"time"
)

var logger *log.Logger

func SetLogger(l *log.Logger) {
	logger = l
}

const ActiveAppBonus = 100 * MB     // 当前激活APP的限制补偿,值越大恢复越快. 但会导致Inactive压力过大
const ActiveAppSWAPRatioInLimit = 3 // 计算ActiveAppLimit的时候会加上(其使用的Swap/此ratio)
const MinimumLimit = 5 * MB         // 内存限制的最小值, 尽量与正常UIAPP的最小值匹配.
const MaximumLimitPlus = 500 * MB   // plus TotalRSSFree, 避免某个UIApp用尽UserSpace的内存,导致僵死无法切换ActiveApp, 从而使swap-sched失效.
const FallbackSamplePeroid = 1      // 默认的数据调整周期
const KernelCacheReserve = 200 * MB //至少预留多少内存给kernel
const DEHardLimit = 800 * MB        // 最多分配给DE多少内存

type Config struct {
	UIAppsCGroup string // sessionID@dde/uiapps
	DECGroup     string // sessionID@dde/DE
	SamplePeroid int    // unit in second // 影响balance采样周期. 值越大系统负载更多
}

type Dispatcher struct {
	sync.Mutex

	cfg Config
	cnt int

	activeXID int

	activeApp    *UIApp
	inactiveApps []*UIApp
	de           *UIApp
}

func NewDispatcher(cfg Config) (*Dispatcher, error) {
	if cfg.SamplePeroid <= 0 {
		cfg.SamplePeroid = FallbackSamplePeroid
	}
	de, err := newApp(cfg.DECGroup, "desktop-environment", DEHardLimit)
	if err != nil {
		return nil, err
	}
	d := &Dispatcher{
		cfg:       cfg,
		cnt:       0,
		activeXID: -1,
		de:        de,
	}

	if err := d.checkCGroups(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Dispatcher) checkCGroups() error {
	groups := []string{
		joinCGPath(memoryCtrl, d.cfg.UIAppsCGroup),
		joinCGPath(freezerCtrl, d.cfg.UIAppsCGroup),

		joinCGPath(memoryCtrl, d.cfg.DECGroup),
		joinCGPath(freezerCtrl, d.cfg.DECGroup),
	}
	for _, path := range groups {
		_, err := os.Stat(path)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) GetDECGroup() string {
	return d.cfg.DECGroup
}

func (d *Dispatcher) counter() int {
	d.Lock()
	d.cnt = d.cnt + 1
	d.Unlock()
	return d.cnt
}

func (d *Dispatcher) NewApp(desktop string, hardLimit uint64) (*UIApp, error) {
	cgroup := fmt.Sprintf("%s/%d", d.cfg.UIAppsCGroup, d.counter())
	app, err := newApp(cgroup, desktop, hardLimit)
	if err != nil {
		return nil, err
	}

	return app, nil
}

func (d *Dispatcher) AddApp(app *UIApp) {
	logger.Debug("Dispatcher.AddApp", app)
	d.Lock()
	d.inactiveApps = append(d.inactiveApps, app)
	d.Unlock()
}

func (d *Dispatcher) setActiveApp(activeApp *UIApp) {
	if d.activeApp == activeApp {
		return
	}

	var inactiveAppsTemp []*UIApp
	if d.activeApp != nil {
		inactiveAppsTemp = append(inactiveAppsTemp, d.activeApp)
	}
	for _, app := range d.inactiveApps {
		if app == activeApp {
			continue
		}
		inactiveAppsTemp = append(inactiveAppsTemp, app)
	}

	d.inactiveApps = inactiveAppsTemp
	d.activeApp = activeApp
}

// sample() 在SamplePeroid的周期下被执行, 所有状态更新的函数都只应该在这里被触发.
func (d *Dispatcher) sample() MemInfo {
	var info MemInfo
	info.TotalRSSFree, info.TotalUsedSwap = getSystemMemoryInfo()
	info.n = len(d.inactiveApps)

	for _, app := range d.inactiveApps {
		app.Update()
		info.InactiveAppsRSS += app.rssUsed
	}

	if d.activeApp != nil {
		d.activeApp.Update()
		info.ActiveAppRSS = d.activeApp.rssUsed
		if info.TotalUsedSwap != 0 {
			info.ActiveAppSWAP = getProcessesSwap(d.activeApp.pids...)
		} else {
			info.ActiveAppSWAP = 0
		}
	}
	d.de.Update()
	return info
}

var debugBalance bool

func init() {
	if os.Getenv("DEBUG_SWAP_SCHED_BALANCE") == "1" {
		debugBalance = true
	}
}

func (d *Dispatcher) balance() {
	info := d.sample()

	if debugBalance {
		if d.activeApp == nil {
			logger.Debugf("no active app (active win: %d)\n%s\n", d.activeXID, info)
		} else {
			logger.Debugf("active app %q(%q) %dMB\n%s\n",
				d.activeApp.desktop,
				d.activeApp.cgroup,
				info.ActiveAppRSS/MB,
				info)
		}
	}

	freezeUIApps(d.cfg.UIAppsCGroup)
	defer thawUIApps(d.cfg.UIAppsCGroup)

	err := setLimitRSS(d.cfg.UIAppsCGroup, info.UIAppsTotalLimit())
	if err != nil {
		logger.Warning("SetUIAppsLimit failed:", err)
	}

	if d.activeApp != nil {
		err = d.activeApp.SetLimitRSS(info.ActiveAppLimit())
		if err != nil {
			logger.Warning("SetActtiveAppLimit failed:", d.activeApp, err)
		}
	}

	var liveApps []*UIApp
	for _, app := range d.inactiveApps {
		if !app.IsLive() {
			logger.Debugf("Dispatcher.balance remove %s from inactiveApps", app)
			continue
		}
		err = app.SetLimitRSS(info.InactiveAppLimit(app.rssUsed))
		if err != nil {
			fmt.Println("SetActtiveAppLimit failed:", app, err)
		}
		liveApps = append(liveApps, app)
	}
	d.inactiveApps = liveApps

	d.de.SetLimitRSS(d.de.rssUsed)
}

func (d *Dispatcher) Balance() {
	delay := time.Second * time.Duration(d.cfg.SamplePeroid)
	for {
		time.Sleep(delay)
		d.Lock()
		d.balance()
		d.Unlock()
	}
}

type MemInfo struct {
	TotalRSSFree  uint64 //当前一共可用的物理内存
	TotalUsedSwap uint64

	ActiveAppRSS    uint64 //ActiveApp占用的物理内存
	ActiveAppSWAP   uint64 //ActiveApp的Swap使用量
	InactiveAppsRSS uint64 //InactiveApps一共占用的物理内存.

	n int
}

// InactiveAppLimit 根据当前可用RSS以及ActiveApp所需RSS计算最小的限制值.
func (info MemInfo) ActiveAppLimit() uint64 {
	swap := info.ActiveAppSWAP / uint64(ActiveAppSWAPRatioInLimit)
	// ActiveApp有大量swap，但RSS较小的情况, 可能会出现inactiveApp反转优先级了．
	// 因此这里加上一定的swap使用量．

	return max(info.ActiveAppRSS+ActiveAppBonus+swap, ActiveAppBonus)
}

// InactiveLimit 根据InactiveApp期望的RSS以及当前可分配的RSS按比例给予.
func (info MemInfo) InactiveAppLimit(desiredRSS uint64) uint64 {
	free := info.TotalRSSFree - info.ActiveAppLimit() - KernelCacheReserve
	if free <= 0 {
		return MinimumLimit
	}
	load := info.InactiveAppsRSS
	return min(max(free*desiredRSS/load, MinimumLimit), desiredRSS)
}

// cgroup uiapps的总限制
func (info MemInfo) UIAppsTotalLimit() uint64 {
	v := info.TotalRSSFree + info.ActiveAppRSS + info.InactiveAppsRSS - KernelCacheReserve
	return max(MinimumLimit, v)
}

func (info MemInfo) String() string {
	str := fmt.Sprintf("TotalFree %dMB, SwapUsed: %dMB\n",
		info.TotalRSSFree/MB, info.TotalUsedSwap/MB)
	str += fmt.Sprintf("UI Limit: %dMB\nActive App Limit: %dMB (need %dMB)\n %d InAcitve Apps need %dMB",
		info.UIAppsTotalLimit()/MB,
		info.ActiveAppLimit()/MB,
		(info.ActiveAppRSS)/MB,
		info.n,
		(info.InactiveAppsRSS)/MB,
	)
	return str
}
