#!/bin/sh

export OS_USERNAME_ENCODED=$(printf ${OS_USERNAME} | base64)
export RAX_API_KEY_ENCODED=$(printf ${RAX_API_KEY} | base64)
envsubst < testdata/rackspace/cert-manager-webhook-rackspace-creds.yml.example > testdata/rackspace/cert-manager-webhook-rackspace-creds.yml
