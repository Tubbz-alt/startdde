/**
 * Copyright (C) 2014 Deepin Technology Co., Ltd.
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 3 of the License, or
 * (at your option) any later version.
 **/

package display

import "github.com/BurntSushi/xgb/xproto"

import "pkg.deepin.io/lib/dbus"

func (dpy *Display) GetDBusInfo() dbus.DBusInfo {
	return dbus.DBusInfo{
		Dest:       "com.deepin.daemon.Display",
		ObjectPath: "/com/deepin/daemon/Display",
		Interface:  "com.deepin.daemon.Display",
	}
}

func (dpy *Display) setPropScreenWidth(v uint16) {
	if dpy.ScreenWidth != v {
		dpy.ScreenWidth = v
		dbus.NotifyChange(dpy, "ScreenWidth")
	}
}

func (dpy *Display) setPropScreenHeight(v uint16) {
	if dpy.ScreenHeight != v {
		dpy.ScreenHeight = v
		dbus.NotifyChange(dpy, "ScreenHeight")
	}
}

func (dpy *Display) setPropPrimaryRect(v xproto.Rectangle) {
	if dpy.PrimaryRect != v {
		dpy.PrimaryRect = v
		dbus.NotifyChange(dpy, "PrimaryRect")
		dbus.Emit(dpy, "PrimaryChanged", dpy.PrimaryRect)
	}
}

func (dpy *Display) setPropPrimary(v string) {
	if dpy.Primary != v {
		dpy.Primary = v
		dbus.NotifyChange(dpy, "Primary")
	}
}

func (dpy *Display) setPropDisplayMode(v int16) {
	dpy.DisplayMode = v
	dbus.NotifyChange(dpy, "DisplayMode")
}

func (dpy *Display) setPropMonitors(v []*Monitor) {
	for _, m := range dpy.Monitors {
		dbus.UnInstallObject(m)
		m = nil
	}

	var tmp []*Monitor
	for _, m := range v {
		err := dbus.InstallOnSession(m)
		if err != nil {
			continue
		}
		tmp = append(tmp, m)
	}
	dpy.Monitors = tmp
	dbus.NotifyChange(dpy, "Monitors")
	dpy.changePrimary(dpy.Primary, false)
}

func (dpy *Display) setPropHasChanged(v bool) {
	if dpy.HasChanged != v {
		dpy.HasChanged = v
		dbus.NotifyChange(dpy, "HasChanged")
	}
}

func (dpy *Display) setPropBrightness(name string, v float64) {
	if old, ok := dpy.Brightness[name]; !ok || old != v {
		dpy.Brightness[name] = v
		dbus.NotifyChange(dpy, "Brightness")
	}
}
