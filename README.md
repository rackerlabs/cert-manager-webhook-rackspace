# cert-manager-webhook-rackspace

A [cert-manager][cert-manager] webhook based [dns01 solver][webhook-solver]
supporting [Rackspace Cloud DNS][raxclouddns].

## Installation

### cert-manager

This package is an addon to [cert-manager][cert-manager] so to get started you
must first have installed it. Instructions can be found [here](https://cert-manager.io/docs/installation/)
for installing it.

### webhook

The webhook must be installed in the same namespace that you've installed cert-manager.
This next step assumes it was `cert-manager`.

```bash
helm install --namespace cert-manager cert-manager-webhook-rackspace charts/cert-manager-webhook-rackspace
```

To uninstall you can run the following:

```bash
helm uninstall --namespace cert-manager cert-manager-webhook-rackspace
```

## Usage

To use the Rackspace Cloud DNS webhook, you must have an account with admin permissions
to the Cloud DNS service and must know the Rackspace API key for that account.

An example secret to provide the credentials would be:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rackspace-creds
type: Opaque
stringData:
  username: my-username-here
  api-key: my-api-key-here
```

Then you can create an `Issuer` or a `ClusterIssuer`. An example would be:

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: letsencrypt-staging
  namespace: cm-test
spec:
  acme:
    # The ACME server URL
    server: https://acme-staging-v02.api.letsencrypt.org/directory

    # Email address used for ACME registration
    email: mail@example.com # REPLACE THIS WITH YOUR EMAIL!!!

    # Name of a secret used to store the ACME account private key
    privateKeySecretRef:
      name: letsencrypt-staging

    solvers:
    - dns01:
        webhook:
          groupName: acme.mycompany.com  # replace with the groupName you set for the helm chart
          solverName: rackspace
          config:
            authSecretRef: rackspace-creds
            domainName: some.domain.tld
```

You can then create a certificate like:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-cert
spec:
  commonName: something.some.domain.tld
  dnsNames:
    - something.some.domain.tld
  issuerRef:
    name: letsencrypt-staging
    kind: Issuer
  secretName: example-cert
```

[cert-manager]: <https://cert-manager.io>
[webhook-solver]: <https://cert-manager.io/docs/configuration/acme/dns01/webhook/>
[raxclouddns]: <https://docs.rackspace.com/docs/cloud-dns>
