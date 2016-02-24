// +build windows

package main

import "log"

func makeDaemon() {
	log.Fatalln("sqs-notify:", "windows doesn't support daemon")
}
