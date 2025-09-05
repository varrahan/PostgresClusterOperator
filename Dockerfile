FROM golang:1.24-alpine AS builder

RUN apk add --no-cache \
    gcc \
    musl-dev \
    libc6-compat \
    make

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -a -o bin/manager main.go

FROM gcr.io/kubebuilder/kube-rbac-proxy:v0.13.0 AS rbacproxy

FROM gcr.io/distroless/static:nonroot

ENV OPERATOR_NAME=postgres-operator \
    OPERATOR_NAMESPACE=postgres-operator-system

COPY --from=builder /workspace/bin/manager /manager
COPY --from=rbacproxy /usr/local/bin/kube-rbac-proxy /usr/local/bin/kube-rbac-proxy

WORKDIR /

USER 65532:65532

ENTRYPOINT ["/manager"]