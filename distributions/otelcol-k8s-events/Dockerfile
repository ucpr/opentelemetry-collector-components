# Build context must be the repo root (not this directory), since the builder-config.yaml's
# `replaces` entry resolves ../../../exporters/notificationexporter relative to the output path
# and needs the whole repo (including go.work) present:
#
#   docker build -f distributions/otelcol-k8s-events/Dockerfile -t otelcol-k8s-events .

FROM golang:1.26-bookworm AS builder
ENV GOTOOLCHAIN=auto
# The repo root's go.work (copied in below) must not apply to the OCB-generated
# _build module: it's a standalone module with its own go.mod/replace directives,
# not a member of that workspace, and Go's workspace auto-detection would
# otherwise refuse to build it from inside the copied repo tree.
ENV GOWORK=off

WORKDIR /src
COPY . .

RUN go install go.opentelemetry.io/collector/cmd/builder@v0.160.0

WORKDIR /src/distributions/otelcol-k8s-events
RUN builder --config=builder-config.yaml

FROM alpine:3.20 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /src/distributions/otelcol-k8s-events/_build/otelcol-k8s-events /otelcol-k8s-events

USER 65534:65534

ENTRYPOINT ["/otelcol-k8s-events"]
CMD ["--config=/etc/otelcol/config.yaml"]
