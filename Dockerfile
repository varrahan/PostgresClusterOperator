# Build stage (Debian-based)
FROM golang:1.24-bullseye AS builder

# Set build arguments
ARG VERSION=1.5.0
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Set working directory
WORKDIR /workspace

# Copy module files first for better caching
COPY go.mod go.mod
COPY go.sum go.sum

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the manager binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -a \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o bin/manager main.go

# Install minimal build tools (Debian packages)
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    git \
    make \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Build kube-rbac-proxy with explicit Go parameters
RUN git clone --depth=1 -b v0.13.0 https://github.com/brancz/kube-rbac-proxy.git /kube-rbac-proxy \
    && cd /kube-rbac-proxy \
    && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 make build \
    && mv kube-rbac-proxy /workspace/bin/ \
    && rm -rf /kube-rbac-proxy

# Final stage
FROM gcr.io/distroless/static:nonroot

# Set environment variables
ENV OPERATOR_NAME=postgres-operator \
    OPERATOR_NAMESPACE=postgres-operator-system

# Copy binaries to standard locations
COPY --from=builder /workspace/bin/manager /manager
COPY --from=builder /workspace/bin/kube-rbac-proxy /usr/local/bin/

# Set working directory
WORKDIR /

# Set user and group (non-root)
USER 65532:65532

# Set entrypoint
ENTRYPOINT ["/manager"]