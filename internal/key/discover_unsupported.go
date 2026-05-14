//go:build !darwin

package key

import "fmt"

const contactRelPath = "contact/contact.db"

type TargetDB struct {
	Account   string
	DataDir   string
	DBRelPath string
	DBPath    string
}

func DiscoverContactDB() (TargetDB, error) {
	return TargetDB{}, fmt.Errorf("automatic WeChat discovery is only implemented for macOS WeChat 4.x in V1")
}
