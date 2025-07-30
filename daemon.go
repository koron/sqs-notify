//go:build !windows
// +build !windows

package main

import (
	"github.com/VividCortex/godaemon"
)

func makeDaemon() {
	godaemon.MakeDaemon(&godaemon.DaemonAttr{})
}
