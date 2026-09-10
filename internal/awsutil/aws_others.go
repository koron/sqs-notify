//go:build !windows
// +build !windows

package awsutil

import (
	"errors"
	"os"
)

func getHomeDir() (string, error) {
	home := os.Getenv("HOME")
	if home != "" {
		return home, nil
	}
	return "", errors.New("could not get HOME")
}
