// +build windows

package main

import "log"

func makeDaemon() {
	log.Fatal("windows doesn't support -daemon")
}
