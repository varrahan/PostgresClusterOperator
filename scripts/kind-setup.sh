#!/bin/bash

set -euo pipefail

PROJECT_ROOT=$(dirname "$(dirname "$(realpath "$0")")")

echo "[1/8] Starting Docker Compose services..."
docker-compose -f "$PROJECT_ROOT/docker-compose.yaml" up -d registry postgres pgadmin

echo "[2/8] Waiting for registry to be ready..."
for i in {1..30}; do
    if curl -f http://localhost:5000/v2/ >/dev/null 2>&1; then
        echo "Registry is ready!"
        break
    fi
    echo "Waiting for registry... (attempt $i/30)"
    sleep 2
done

echo "[3/8] Deleting old Kind cluster (if exists)..."
kind delete cluster --name postgres-operator || true

echo "[4/8] Creating new Kind cluster..."
kind create cluster --name postgres-operator --config "$PROJECT_ROOT/kind-config.yaml"

echo "[5/8] Installing cert-manager..."
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml
kubectl wait --for=condition=Available -n cert-manager deployment/cert-manager-webhook --timeout=300s

echo "[6/8] Building and pushing operator image to local registry..."
docker build -t localhost:5000/postgres-operator:latest "$PROJECT_ROOT"

echo "Pushing image to localhost:5000..."
for i in {1..5}; do
    if docker push localhost:5000/postgres-operator:latest; then
        echo "Image pushed successfully!"
        break
    fi
    echo "Push failed, retrying... (attempt $i/5)"
    sleep 3
done

echo "[7/8] Installing CRDs and deploying operator..."
make -C "$PROJECT_ROOT" install

kubectl create ns postgres-operator-system --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -k "$PROJECT_ROOT/config/certmanager"
kubectl wait --for=condition=Ready -n postgres-operator-system certificate/webhook-server-cert --timeout=120s

kubectl kustomize "$PROJECT_ROOT/config/default" | kubectl apply -f -
kubectl rollout status deployment -n postgres-operator-system postgres-operator-controller-manager --timeout=120s

echo "[8/8] Applying sample resources..."
kubectl apply -f "$PROJECT_ROOT/config/samples/database_v1_postgrescluster.yaml"
kubectl create secret generic pg-user-secret --from-literal=password=securepassword123 --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f "$PROJECT_ROOT/config/samples/database_v1_postgresuser.yaml"

echo -e "\nSetup complete! Verify with:"
echo "  kubectl get postgresclusters"
echo "  kubectl get pods -n postgres-operator-system"
echo "  kubectl get postgresusers"
echo -e "\nRegistry available at: http://localhost:5000/v2/_catalog"
echo "PostgreSQL available at: localhost:5432 (admin/admin)"
echo "PgAdmin available at: http://localhost:8080 (admin@example.com/admin)"