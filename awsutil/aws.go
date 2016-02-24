package awsutil

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/goamz/goamz/aws"
	"github.com/vaughan0/go-ini"
)

func loadCredFile() (ini.File, error) {
	credFile := os.Getenv("AWS_CREDENTIAL_FILE")
	if credFile == "" {
		home, err := getHomeDir()
		if err != nil {
			return nil, err
		}
		credFile = filepath.Join(home, ".aws", "credentials")
	}
	return ini.LoadFile(credFile)
}

// GetAuth returns aws.Auth from credentials or envrionment variables.
func GetAuth(name string) (aws.Auth, error) {
	f, err := loadCredFile()
	if err != nil {
		if os.IsExist(err) {
			return aws.Auth{}, err
		}
		return aws.EnvAuth()
	}

	var prof ini.Section
	var ok bool

	if name != "" {
		prof, ok = f[name]
	}
	if !ok {
		prof, ok = f["default"]
	}
	if !ok {
		return aws.Auth{}, errors.New("cannot find section")
	}

	// Parse auth info from a ini's section.
	a := aws.Auth{
		AccessKey: prof["aws_access_key_id"],
		SecretKey: prof["aws_secret_access_key"],
	}
	if a.AccessKey == "" {
		return aws.Auth{}, errors.New("empty aws_access_key_id in credentials")
	}
	if a.SecretKey == "" {
		return aws.Auth{}, errors.New("empty aws_secret_access_key in credentials")
	}
	return a, nil
}
