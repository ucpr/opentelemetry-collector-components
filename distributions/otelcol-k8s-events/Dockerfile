# Build context must be the repo root (not this directory), since the builder-config.yaml's
# `replaces` entry resolves ../../../exporters/notificationexporter relative to the output path
# and needs the whole repo (including go.work) present:
#
#   docker build -f distributions/otelcol-k8s-events/Dockerfile -t otelcol-k8s-events .

FROM golang:1.26-bookworm AS builder
ENV GOTOOLCHAIN=auto

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
