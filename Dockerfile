# goreleaser is making the binary dynamically linked so can't use the static container
FROM gcr.io/distroless/base-debian12:nonroot

USER 20000:20000
COPY --chmod=555 cert-manager-webhook-rackspace /usr/local/bin/cert-manager-webhook-rackspace
ENTRYPOINT ["/usr/local/bin/cert-manager-webhook-rackspace"]