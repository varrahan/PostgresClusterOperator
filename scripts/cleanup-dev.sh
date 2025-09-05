#!/bin/bash

# Get project root directory
PROJECT_ROOT=$(dirname $(dirname $(realpath $0)))

# Default values
REMOVE_CLUSTER=true
REMOVE_STORAGE=false
REMOVE_IMAGE=false
REMOVE_CONFIG=false

# Parse command-line options
while [[ $# -gt 0 ]]; do
    case "$1" in
        --all)
            REMOVE_STORAGE=true
            REMOVE_IMAGE=true
            REMOVE_CONFIG=true
            shift
            ;;
        --storage)
            REMOVE_STORAGE=true
            shift
            ;;
        --image)
            REMOVE_IMAGE=true
            shift
            ;;
        --config)
            REMOVE_CONFIG=true
            shift
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  --all       Remove everything (cluster, storage, image, config)"
            echo "  --storage   Remove local storage data"
            echo "  --image     Remove operator Docker image"
            echo "  --config    Remove Kind configuration (kind-config.yaml, local-secret.yaml)"
            echo "  --help      Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Remove cluster
if [ "$REMOVE_CLUSTER" = true ]; then
    echo "Removing Kind cluster..."
    kind delete cluster --name postgres-operator 2>/dev/null || true
    
    # Force remove any remaining containers
    docker rm -f registry
    docker rm -f $(docker ps -aq -f "name=postgres-operator") 2>/dev/null || true
fi

# Remove storage
if [ "$REMOVE_STORAGE" = true ]; then
    echo "Cleaning local storage..."
    rm -rf ${PROJECT_ROOT}/local-storage/* 2>/dev/null || true
fi

# Remove Docker image
if [ "$REMOVE_IMAGE" = true ]; then
    echo "Removing Docker image..."
    docker rmi -f postgres-operator:latest 2>/dev/null || true
    docker rm -f $(docker ps -aq -f "name=postgres") 2>/dev/null || true
    docker rm -f $(docker ps -aq -f "name=pgadmin") 2>/dev/null || true
fi

# Remove config files
if [ "$REMOVE_CONFIG" = true ]; then
    echo "Removing config files..."
    rm -f ${PROJECT_ROOT}/kind-config.yaml 2>/dev/null || true
    rm -f ${PROJECT_ROOT}/config/samples/local-secret.yaml 2>/dev/null || true
fi

echo "Cleanup complete!"