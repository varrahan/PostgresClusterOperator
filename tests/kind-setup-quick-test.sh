#!/bin/bash

GREEN=$'\033[0;32m\u2714'
RED=$'\033[0;31m\u274C'
WHITE=$'\033[0;37m\u2714'

echo "=== Checking if namespace exists ==="
kubectl get namespace postgres-operator-system

echo ""
echo "=== Testing kustomize build ==="
kubectl kustomize config/default

echo ""
echo "=== Testing kustomize apply with dry-run ==="
kubectl kustomize config/default | kubectl apply --dry-run=client -f - && echo "${GREEN} kustomize apply OK" || echo "${RED} kustomize apply FAILED"

echo "${WHITE}"
echo "=== Checking what's actually in the namespace ==="
kubectl get all -n postgres-operator-system

echo "=== Testing individual components ==="
echo "Testing CRDs..."
kubectl kustomize config/crd | kubectl apply --dry-run=client -f - && echo "${GREEN} CRDs OK" || echo "${RED} CRDs FAILED"

echo "${WHITE}Testing RBAC..."
kubectl kustomize config/rbac | kubectl apply --dry-run=client -f - && echo "${GREEN} RBAC OK" || echo "${RED} RBAC FAILED"

echo "${WHITE}Testing Manager..."
kubectl kustomize config/manager | kubectl apply --dry-run=client -f - && echo "${GREEN} Manager OK" || echo "${RED} Manager FAILED"

echo "${WHITE}"
echo "=== Checking certificate status ==="
kubectl get certificates -n postgres-operator-system

echo ""
echo "=== Checking if certmanager resources exist ==="
kubectl get all -n cert-manager