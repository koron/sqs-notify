// +build windows

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
	home = os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
	if home != "" {
		return home, nil
	}
	home = os.Getenv("USERPROFILE")
	if home != "" {
		return home, nil
	}
	return "", errors.New("could not get HOME")
}
