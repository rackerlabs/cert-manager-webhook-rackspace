package internal

import (
	"github.com/gophercloud/gophercloud"
)

type Config struct {
	DomainName string
	Service    *gophercloud.ServiceClient
}
