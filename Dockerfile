# Build the manager binary
FROM registry.access.redhat.com/ubi8/go-toolset:1.22.9-3.1742991062 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the Go source (relies on .dockerignore to filter)
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

FROM registry.access.redhat.com/ubi8/ubi-minimal:8.10-1216
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65534:65534

ENTRYPOINT ["/manager"]
