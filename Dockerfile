# syntax=docker/dockerfile:1

# Builder always runs on the host's native architecture (--platform=$BUILDPLATFORM)
# and cross-compiles for the target via GOOS/GOARCH, so multi-arch builds never
# need to emulate the Go toolchain under QEMU.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
    -ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.buildTime=$BUILD_TIME" \
    -o /out/cf-smtp-relay ./cmd/cf-smtp-relay

# static-debian12:nonroot already runs as uid 65532, ships the CA bundle
# needed for the HTTPS call to Cloudflare, and has no shell — no extra
# hardening steps needed.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/cf-smtp-relay /cf-smtp-relay
EXPOSE 2525
ENTRYPOINT ["/cf-smtp-relay"]
CMD ["serve"]
