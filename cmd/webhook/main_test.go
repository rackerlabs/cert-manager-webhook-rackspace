package main

import (
	"math/rand"
	"os"
	"testing"
	"time"

	acmetest "github.com/cert-manager/cert-manager/test/acme"
)

var (
	zone = os.Getenv("TEST_ZONE_NAME")
	fqdn string
)

func TestRunsSuite(t *testing.T) {
	// The manifest path should contain a file named config.json that is a
	// snippet of valid configuration that should be included on the
	// ChallengeRequest passed as part of the test cases.
	//

	// increase polling time to 10 seconds
	pollTime, _ := time.ParseDuration("10s")
	// increase propogation timeout to 6 minutes
    // since the Rackspace minimum is 5 minutes
	timeOut, _ := time.ParseDuration("6m")

	// create a unique domain to test against
	fqdn = GetRandomString(20) + "." + zone

	// Uncomment the below fixture when implementing your custom DNS provider
	fixture := acmetest.NewFixture(&rackspaceDNSProviderSolver{},
		acmetest.SetResolvedZone(zone),
		acmetest.SetResolvedFQDN(fqdn),
		acmetest.SetAllowAmbientCredentials(false),
		acmetest.SetManifestPath("../../testdata/rackspace"),
		acmetest.SetPollInterval(pollTime),
		acmetest.SetPropagationLimit(timeOut),
	)
	fixture.RunConformance(t)

}

func GetRandomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
