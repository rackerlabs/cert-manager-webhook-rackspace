package internal

import "github.com/rackerlabs/goraxauth"

type Config struct {
	DomainName  string
	AuthOptions goraxauth.AuthOptions
}
