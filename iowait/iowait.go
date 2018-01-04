/*
 * Copyright (C) 2018 ~ 2018 Deepin Technology Co., Ltd.
 *
 * Author:     kirigaya <kirigaya@mkacg.com>
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package iowait

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"pkg.deepin.io/lib/log"
	"pkg.deepin.io/lib/strv"
)

// #cgo pkg-config: x11 xcursor xfixes gio-2.0
// #include "xcursor_remap.h"
import "C"

var (
	step       int
	ioWaitStep float64
	_logger    *log.Logger
	cpuState   CPUStat
)

type CPUStat struct {
	System float64
	Idle   float64
	IOWait float64
	Count  float64
}

func Start(logger *log.Logger) {
	_logger = logger
	step = 0
	ticker := time.NewTicker(time.Second * 2)
	for {
		select {
		case <-ticker.C:
			showIOWait()
		}
	}
}

func showIOWait() {
	fr, err := os.Open("/proc/stat")
	if err != nil {
		_logger.Warning("Failed to open:", err)
		return
	}

	var scanner = bufio.NewScanner(fr)
	scanner.Scan()
	line := scanner.Text()
	fr.Close()
	list := strings.Split(line, " ")
	list = strv.Strv(list).FilterEmpty()
	if len(list) < 6 {
		_logger.Warning("INvalid format:", line, len(list))
		return
	}

	var TEMP CPUStat

	TEMP.System = stof(list[3])
	TEMP.Idle = stof(list[4])
	TEMP.IOWait = stof(list[5])
	TEMP.Count = (TEMP.System + TEMP.Idle + TEMP.IOWait)

	var tempStep = 100.0 * (TEMP.IOWait - cpuState.IOWait) / (TEMP.Count - cpuState.Count)

	_logger.Debug("current info: ", TEMP)
	_logger.Debug("last info: ", cpuState)
	_logger.Debug("current step: ", tempStep)
	_logger.Debug("last step: ", ioWaitStep)
	_logger.Debug("step: ", step)

	if (tempStep >= 75 && ioWaitStep >= 75) || (tempStep <= 75 && ioWaitStep <= 75) {
		if step == 5 {
			xcLeftPtrToWatch(tempStep >= 75)
			step = 0
		} else {
			step++
		}
	} else {
		step = 0
	}

	cpuState = TEMP
	ioWaitStep = tempStep
}

func stof(v string) float64 {
	r, _ := strconv.ParseFloat(v, 64)
	return r
}

func xcLeftPtrToWatch(enabled bool) {
	var v C.int = 1
	if !enabled {
		v = 0
	}

	ret := C.xc_left_ptr_to_watch(v)
	if ret != 0 {
		fmt.Printf("Failed to map(%v) left_ptr/watch", enabled)
	}
}
