# Kubernetes Postgres Cluster Operator

## 📖 Overview

The **Postgres Cluster Operator** is a Kubernetes Operator designed to simplify the deployment, management, and scaling of PostgreSQL clusters within a Kubernetes environment.

This tool allows PostgreSQL to be **safely managed and scaled** inside a Kubernetes cluster by leveraging Custom Resources (CRDs) and the Kubernetes controller pattern.

## ✨ Features

Based on common Operator functionality and the repository structure, this Operator is likely capable of:

* **Automated Deployment:** Deploying a highly available PostgreSQL cluster with primary and replica instances.
* **Safe Failover:** Automated detection and promotion of a new primary instance upon failure of the current one.
* **Replication Management:** Configuring and maintaining streaming replication between the primary and replicas.
* **Scaling:** Easy horizontal scaling of read replicas.
* **Custom Resource Definition (CRD):** Manages the lifecycle of Postgres clusters via a simple `PostgresCluster` custom resource.
* **Configuration:** Customization of PostgreSQL parameters and persistent storage options.

## 🚀 Getting Started

These instructions will get a copy of the Operator running on your local Kubernetes cluster using **kind**.

### Prerequisites

* A running Kubernetes cluster (v1.24+ recommended).
* `kubectl` installed and configured.
* `kustomize` for manifest generation.
* `make` and `go` (if building from source).

### Installation

#### 1. Deploy the Operator

You can deploy the operator and its necessary Custom Resource Definitions (CRDs) using `kustomize`:

```bash
# Apply the CRDs to your cluster
kubectl apply -k config/crd/

# Apply the operator and RBAC configurations
kubectl apply -k config/default
```
Alternatively, if you are developing or testing, you can deploy a local image using make:

```bash
# Build and push the image (you may need to adjust the Makefile for your registry)
make docker-build docker-push

# Deploy the operator (assuming image is accessible)
make deploy
```
#### 2. Verify the Installation
Check that the operator pod is running in the default namespace (or the namespace you configured):

```bash
kubectl get pods -l control-plane=controller-manager
```

You should see an output similar to:

```bash
NAME                                   READY   STATUS    RESTARTS   AGE
postgrescluster-operator-controller-manager-...   1/1     Running   0          5m
```

## 🛠️ Usage

Once the operator is running, you can create your first PostgreSQL cluster by defining a `PostgresCluster` Custom Resource.

### Example `PostgresCluster` Resource

Save the following content as `my-postgres-cluster.yaml`:

```yaml
apiVersion: postgres.varrahan.io/v1
kind: PostgresCluster
metadata:
  name: my-first-cluster
spec:
  # The desired number of replicas (excluding the primary)
  replicas: 2
  
  # Configuration for the PostgreSQL primary and replica
  postgres:
    version: 16
    image: registry.opensource.zalan.do/acid/postgres-operator:v1.10.0-p1 # Replace with your preferred image
    
  # Resource requests/limits for the containers
  resources:
    requests:
      cpu: "100m"
      memory: "256Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
      
  # Volume claims for persistent storage
  volume:
    size: 10Gi
    storageClass: standard # Replace with your cluster's StorageClass name
```

### Apply the Custom Resource

Create the cluster by applying the file:

```bash
kubectl apply -f my-postgres-cluster.yaml
```

The operator will now watch for this resource and automatically create the necessary StatefulSets, Services, and Secrets to deploy and manage your Postgres cluster.

### Accessing the Cluster

The operator typically creates a Kubernetes Service to allow connections.

```bash
# Get services to find the primary connection endpoint
kubectl get svc -l postgres-cluster=my-first-cluster
```
