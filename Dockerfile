# Build the manager binary
FROM golang:1.16 as builder
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
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager cmd/operator/operator.go

FROM alpine:3.14.0
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]