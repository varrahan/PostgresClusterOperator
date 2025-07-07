# Build stage
FROM golang:1.21-alpine AS builder

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

# Build the kube-rbac-proxy (for secure metrics)
RUN git clone --depth=1 -b v0.13.0 https://github.com/brancz/kube-rbac-proxy.git /kube-rbac-proxy && \
    cd /kube-rbac-proxy && \
    make build && \
    mv kube-rbac-proxy /workspace/bin/

# Final stage
FROM gcr.io/distroless/static:nonroot

# Set environment variables
ENV OPERATOR_NAME=postgres-operator \
    OPERATOR_NAMESPACE=postgres-operator-system

# Copy binaries
COPY --from=builder /workspace/bin/manager /manager
COPY --from=builder /workspace/bin/kube-rbac-proxy /usr/local/bin/kube-rbac-proxy

# Set working directory
WORKDIR /

# Set user and group (non-root)
USER 65532:65532

# Set entrypoint
ENTRYPOINT ["/manager"]