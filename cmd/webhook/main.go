package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"k8s.io/klog/v2"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"

	"github.com/gophercloud/gophercloud/v2"
	tokens2 "github.com/gophercloud/gophercloud/v2/openstack/identity/v2/tokens"
	"github.com/gophercloud/gophercloud/v2/pagination"

	"github.com/rackerlabs/cert-manager-webhook-rackspace/internal"
	"github.com/rackerlabs/goclouddns"
	"github.com/rackerlabs/goclouddns/domains"
	"github.com/rackerlabs/goclouddns/records"
	"github.com/rackerlabs/goraxauth"
)

var GroupName = os.Getenv("GROUP_NAME")

const banner = `
%s
version: %s (%s)

`

const SelfName = "cert-manager-webhook-rackspace"

var (
	Version = "local"
	Gitsha  = "?"
)

func main() {
	fmt.Printf(banner, SelfName, Version, Gitsha)

	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&rackspaceDNSProviderSolver{},
	)
}

// rackspaceDNSProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/cert-manager/cert-manager/pkg/acme/webhook.Solver`
// interface.
type rackspaceDNSProviderSolver struct {
	// If a Kubernetes 'clientset' is needed, you must:
	// 1. uncomment the additional `client` field in this structure below
	// 2. uncomment the "k8s.io/client-go/kubernetes" import at the top of the file
	// 3. uncomment the relevant code in the Initialize method below
	// 4. ensure your webhook's service account has the required RBAC role
	//    assigned to it for interacting with the Kubernetes APIs you need.
	client *kubernetes.Clientset
}

// rackspaceDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
// This typically includes references to Secret resources containing DNS
// provider credentials, in cases where a 'multi-tenant' DNS solver is being
// created.
// If you do *not* require per-issuer or per-certificate configuration to be
// provided to your webhook, you can skip decoding altogether in favour of
// using CLI flags or similar to provide configuration.
// You should not include sensitive information here. If credentials need to
// be used by your provider here, you should reference a Kubernetes Secret
// resource and fetch these credentials using a Kubernetes clientset.
type rackspaceDNSProviderConfig struct {
	DomainName    string `json:"domainName"`
	AuthSecretRef string `json:"authSecretRef"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (c *rackspaceDNSProviderSolver) Name() string {
	return "rackspace"
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (c *rackspaceDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	klog.V(6).Infof("call function Present: namespace=%s, zone=%s, fqdn=%s",
		ch.ResourceNamespace, ch.ResolvedZone, ch.ResolvedFQDN)

	timeout := 60 * time.Second
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	cfg, err := clientConfig(c, ch)
	if err != nil {
		return fmt.Errorf("unable to get secret from namespace `%s`: %w", ch.ResourceNamespace, err)
	}

	service, err := authenticateClient(ctx, cfg)
	if err != nil {
		return fmt.Errorf("unable to authenticate to rackspace: %w", err)
	}

	klog.Infof("Configured Rackspace Cloud DNS client")

	// Rackspace will create any case but reply back with lower case, it doesn't like trailing dots
	// so make our calls consistent on create and lookup/delete
	domainName := strings.ToLower(strings.TrimSuffix(ch.ResolvedZone, "."))
	fqdn := strings.ToLower(strings.TrimSuffix(ch.ResolvedFQDN, "."))

	domId, err := loadDomainId(ctx, service, domainName)
	if err != nil {
		return fmt.Errorf("unable to find domain ID for domain `%s`: %w", ch.ResolvedZone, err)
	}

	opts := records.CreateOpts{
		Name:    fqdn,
		Type:    "TXT",
		Data:    ch.Key,
		TTL:     300,
		Comment: fmt.Sprintf("created by %s/%s", SelfName, Version),
	}

	record, err := records.Create(ctx, service, domId, opts).Extract()
	if err != nil {
		return fmt.Errorf("unable to create DNS record `%v`: %w", ch.ResolvedFQDN, err)
	}

	klog.Infof("Presented txt record %v as %v", ch.ResolvedFQDN, record)

	return nil
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (c *rackspaceDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	klog.V(6).Infof("call function CleanUp: namespace=%s, zone=%s, fqdn=%s",
		ch.ResourceNamespace, ch.ResolvedZone, ch.ResolvedFQDN)

	timeout := 60 * time.Second
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()

	cfg, err := clientConfig(c, ch)
	if err != nil {
		return fmt.Errorf("unable to get secret from namespace `%s`: %w", ch.ResourceNamespace, err)
	}

	service, err := authenticateClient(ctx, cfg)
	if err != nil {
		return fmt.Errorf("unable to authenticate to rackspace: %w", err)
	}

	klog.Infof("Configured Rackspace Cloud DNS client")

	// Rackspace will create any case but reply back with lower case, it doesn't like trailing dots
	// so make our calls consistent on create and lookup/delete
	domainName := strings.ToLower(strings.TrimSuffix(ch.ResolvedZone, "."))

	domId, err := loadDomainId(ctx, service, domainName)
	if err != nil {
		return fmt.Errorf("unable to find domain ID for domain `%s`: %w", ch.ResolvedZone, err)
	}

	recordId, err := loadRecordId(ctx, service, domId, ch)
	if err != nil {
		return fmt.Errorf("unable to find DNS record for `%s`: %w", ch.ResolvedFQDN, err)
	}

	deleteErr := records.Delete(ctx, service, domId, recordId).ExtractErr()
	if deleteErr != nil {
		return fmt.Errorf("unable to delete DNS record for `%s`: %w", ch.ResolvedFQDN, err)
	}

	klog.Infof("Deleted txt record %v", ch.ResolvedFQDN)

	return nil
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (c *rackspaceDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	klog.V(6).Infof("Input variable stopCh is %d length", len(stopCh))
	if err != nil {
		return err
	}

	c.client = cl

	return nil
}

func stringFromSecretData(secretData map[string][]byte, key string) (string, error) {
	data, ok := secretData[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret data", key)
	}
	return string(data), nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (rackspaceDNSProviderConfig, error) {
	cfg := rackspaceDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %w", err)
	}

	return cfg, nil
}

func clientConfig(c *rackspaceDNSProviderSolver, ch *v1alpha1.ChallengeRequest) (internal.Config, error) {
	var config internal.Config

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return config, err
	}

	secretName := cfg.AuthSecretRef
	sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})

	if err != nil {
		return config, fmt.Errorf("unable to get secret `%s/%s`: %w", ch.ResourceNamespace, secretName, err)
	}

	username, err := stringFromSecretData(sec.Data, "username")
	if err != nil {
		return config, fmt.Errorf("unable to get username from secret `%s/%s`: %w", ch.ResourceNamespace, secretName, err)
	}

	apiKey, err := stringFromSecretData(sec.Data, "api-key")
	if err != nil {
		return config, fmt.Errorf("unable to get api-key from secret `%s/%s`: %w", ch.ResourceNamespace, secretName, err)
	}

	ao := goraxauth.AuthOptions{
		AuthOptions: tokens2.AuthOptions{
			IdentityEndpoint: "https://identity.api.rackspacecloud.com/v2.0/",
			Username:         username,
		},
		ApiKey: apiKey,
	}

	config.DomainName = cfg.DomainName
	config.AuthOptions = ao

	return config, nil
}

func authenticateClient(ctx context.Context, c internal.Config) (*gophercloud.ServiceClient, error) {
	provider, err := goraxauth.AuthenticatedClient(ctx, c.AuthOptions)
	if err != nil {
		return nil, fmt.Errorf("unable to authenticate to rackspace as `%s`: %w", c.AuthOptions.Username, err)
	}

	provider.UserAgent.Prepend(SelfName, "/", Version)

	service, err := goclouddns.NewCloudDNS(provider, gophercloud.EndpointOpts{})
	if err != nil {
		return nil, fmt.Errorf("unable to find cloud dns endpoint for rackspace as `%s`: %w", c.AuthOptions.Username, err)
	}

	return service, nil
}

func loadDomainId(ctx context.Context, service *gophercloud.ServiceClient, domainName string) (string, error) {
	var domId string

	opts := domains.ListOpts{
		Name: domainName,
	}

	pager := domains.List(ctx, service, opts)

	listErr := pager.EachPage(ctx, func(ctx context.Context, page pagination.Page) (bool, error) {
		domainList, err := domains.ExtractDomains(page)

		if err != nil {
			return false, err
		}

		if len(domainList) == 0 {
			return false, fmt.Errorf("failed to find domain for `%s`", domainName)
		}

		for _, domain := range domainList {
			if domain.Name == domainName {
				domId = domain.ID
				return true, err
			}
		}

		// go to the next page
		return true, err
	})

	if listErr != nil {
		return domId, fmt.Errorf("unable to fetch domains in rackspace account: %w", listErr)
	}

	if domId == "" {
		return domId, fmt.Errorf("failed to find domain `%s`", domainName)
	}

	return domId, nil
}

func loadRecordId(ctx context.Context, service *gophercloud.ServiceClient, domId string, ch *v1alpha1.ChallengeRequest) (string, error) {
	var recordId string

	// Rackspace will create any case but reply back with lower case, it doesn't like trailing dots
	// so make our calls consistent on create and lookup/delete
	fqdn := strings.ToLower(strings.TrimSuffix(ch.ResolvedFQDN, "."))

	opts := records.ListOpts{
		Name: fqdn,
		Type: "TXT",
		Data: ch.Key,
	}

	pager := records.List(ctx, service, domId, opts)

	listErr := pager.EachPage(ctx, func(ctx context.Context, page pagination.Page) (bool, error) {
		recordList, err := records.ExtractRecords(page)

		if err != nil {
			return false, err
		}

		if len(recordList) > 1 {
			return false, fmt.Errorf("multiple records matched `%s`, manual cleanup required. count %d", ch.ResolvedFQDN, len(recordList))
		}

		if len(recordList) == 0 {
			return false, fmt.Errorf("failed to find record for `%s`", ch.ResolvedFQDN)
		}

		recordId = recordList[0].ID
		return true, err
	})

	if listErr != nil {
		return "", fmt.Errorf("unable to fetch DNS records in rackspace account: %w", listErr)
	}

	if recordId == "" {
		return "", fmt.Errorf("failed to find DNS record `%s`", ch.ResolvedFQDN)
	}

	return recordId, nil
}
