#!/bin/bash

PROJECT_ROOT=$(dirname $(dirname $(realpath $0)))

kind create cluster --name postgres-operator --config ${PROJECT_ROOT}/kind-config.yaml

docker build -t postgres-operator:latest ..
kind load docker-image postgres-operator:latest --name postgres-operator

make install

make deploy IMG=postgres-operator:latest

kubectl wait --for=condition=available deployment/postgres-operator-controller-manager -n postgres-operator-system --timeout=120s

kubectl apply -f ${PROJECT_ROOT}/config/samples/database_v1_postgrescluster.yaml
kubectl create secret generic pg-user-secret --from-literal=password=securepassword123
kubectl apply -f ${PROJECT_ROOT}/config/samples/database_v1_postgresuser.yaml

echo "Setup complete! Use 'kubectl get postgresclusters' to check status"