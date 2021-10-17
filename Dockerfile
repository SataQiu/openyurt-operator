# Build the manager binary
FROM --platform=${BUILDPLATFORM} golang:1.16 as builder

ARG TARGETOS
ARG TARGETARCH

ARG GO_PROXY
ENV GOPROXY ${GO_PROXY:-"https://proxy.golang.org,direct"}

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY api ./api
COPY cmd ./cmd
COPY pkg ./pkg

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GO111MODULE=on go build -a -o manager cmd/manager/manager.go

FROM alpine:3.12.0
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]
