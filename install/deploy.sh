#!/bin/bash

# Deploy script for Thunder with PostgreSQL and Kubernetes
# This script handles setup and cleanup of all resources

set -e

# Configuration
NAMESPACE="thunder-setup"
CONFIGMAP_NAME="thunder-bootstrap"
HELM_RELEASE="thunder"
DOCKER_COMPOSE_FILE="local-development/docker-compose.yml"
# ANALYTICS_COMPOSE_FILE="analytics/docker-compose.yaml"
# BOOTSTRAP_DIR and RESOURCES_BASE_DIR are set based on mode below
HELM_CHART_DIR="helm"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Log functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Mode selection
MODE=${1:-demo-app}

if [ "$MODE" == "demo-app" ]; then
    BOOTSTRAP_DIR="helm/demo-app/boostrap"
    RESOURCES_BASE_DIR="helm/demo-app/resources"
elif [ "$MODE" == "demo-scale" ]; then
    BOOTSTRAP_DIR="helm/demo-scale/boostrap"
    RESOURCES_BASE_DIR="helm/demo-scale/resources"
else
    log_warn "Invalid mode: $MODE. Available modes: demo-app, demo-scale"
    # Using log_warn because log_error might not exit, but here we exit manually
    echo -e "${RED}[ERROR]${NC} Invalid mode. Exiting."
    exit 1
fi

log_info "Deploying in mode: $MODE"

# Cleanup function - called on script exit or cancellation
cleanup() {
    log_warn "Cleaning up resources..."
    
    # Uninstall Helm release
    if helm list -n "$NAMESPACE" | grep -q "$HELM_RELEASE"; then
        log_info "Uninstalling Helm release: $HELM_RELEASE"
        helm uninstall "$HELM_RELEASE" -n "$NAMESPACE" || log_warn "Failed to uninstall Helm release"
    fi
    
    # Delete all ConfigMaps
    log_info "Deleting ConfigMaps..."
    for cm in "$CONFIGMAP_NAME" "thunder-applications" "thunder-flows" "thunder-organization-units" "thunder-user-schemas" "thunder-identity-providers"; do
        if kubectl get configmap "$cm" -n "$NAMESPACE" &>/dev/null; then
            kubectl delete configmap "$cm" -n "$NAMESPACE" || log_warn "Failed to delete ConfigMap $cm"
        fi
    done
    
    # Delete namespace
    if kubectl get namespace "$NAMESPACE" &>/dev/null; then
        log_info "Deleting namespace: $NAMESPACE"
        kubectl delete namespace "$NAMESPACE" || log_warn "Failed to delete namespace"
    fi
    
    # Stop Docker Compose services
    if docker ps | grep -q "local_postgres"; then
        log_info "Stopping PostgreSQL Docker Compose services..."
        docker-compose -f "$DOCKER_COMPOSE_FILE" down || log_warn "Failed to stop PostgreSQL Docker Compose"
    fi
    
    # if [ -f "$ANALYTICS_COMPOSE_FILE" ]; then
    #     log_info "Stopping Analytics Docker Compose services..."
    #     docker-compose -f "$ANALYTICS_COMPOSE_FILE" down || log_warn "Failed to stop Analytics Docker Compose"
    # fi
    
    log_info "Cleanup completed!"
}

# Trap signals for cleanup
trap cleanup EXIT INT TERM

# Main deployment function
deploy() {
    log_info "Starting Thunder deployment..."
    
    # 1. Start PostgreSQL with Docker Compose
    log_info "Starting PostgreSQL with Docker Compose..."
    if [ ! -f "$DOCKER_COMPOSE_FILE" ]; then
        log_error "Docker Compose file not found: $DOCKER_COMPOSE_FILE"
        exit 1
    fi
    docker-compose -f "$DOCKER_COMPOSE_FILE" up -d
    
    # Wait for PostgreSQL to be ready
    log_info "Waiting for PostgreSQL to be ready..."
    sleep 5
    while ! docker exec local_postgres pg_isready -U asgthunder &>/dev/null; do
        log_info "Waiting for PostgreSQL..."
        sleep 2
    done
    log_info "PostgreSQL is ready!"
    
    # # Start Analytics with Docker Compose
    # log_info "Starting Analytics with Docker Compose..."
    # if [ ! -f "$ANALYTICS_COMPOSE_FILE" ]; then
    #     log_warn "Analytics Docker Compose file not found: $ANALYTICS_COMPOSE_FILE"
    # else
    #     docker-compose -f "$ANALYTICS_COMPOSE_FILE" up -d
    #     log_info "Analytics services started!"
    # fi
    
    # 2. Create Kubernetes namespace
    log_info "Creating namespace: $NAMESPACE"
    if kubectl get namespace "$NAMESPACE" &>/dev/null; then
        log_warn "Namespace $NAMESPACE already exists, skipping creation"
    else
        kubectl create namespace "$NAMESPACE"
    fi
    
    # 3. Create ConfigMap for bootstrap scripts
    log_info "Creating ConfigMap: $CONFIGMAP_NAME"
    if [ ! -d "$BOOTSTRAP_DIR" ]; then
        log_error "Bootstrap directory not found: $BOOTSTRAP_DIR"
        exit 1
    fi
    
    if kubectl get configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" &>/dev/null; then
        log_warn "ConfigMap $CONFIGMAP_NAME already exists, deleting and recreating..."
        kubectl delete configmap "$CONFIGMAP_NAME" -n "$NAMESPACE"
    fi
    
    kubectl create configmap "$CONFIGMAP_NAME" \
        --from-file="$BOOTSTRAP_DIR" \
        --namespace="$NAMESPACE"
    
    log_info "ConfigMap created successfully!"
    
    # 3b. Create ConfigMaps for resource directories
    create_resource_configmap() {
        local name=$1
        local dir=$2
        local cm_name="thunder-$name"
        
        if [ -d "$dir" ] && [ "$(ls -A $dir 2>/dev/null)" ]; then
            if kubectl get configmap "$cm_name" -n "$NAMESPACE" &>/dev/null; then
                log_info "ConfigMap $cm_name already exists, deleting and recreating..."
                kubectl delete configmap "$cm_name" -n "$NAMESPACE"
            fi
            log_info "Creating ConfigMap: $cm_name from $dir"
            kubectl create configmap "$cm_name" \
                --from-file="$dir" \
                --namespace="$NAMESPACE"
        else
            log_warn "Directory $dir is empty or does not exist, skipping ConfigMap $cm_name"
        fi
    }
    
    create_resource_configmap "applications" "$RESOURCES_BASE_DIR/applications"
    create_resource_configmap "flows" "$RESOURCES_BASE_DIR/flows"
    create_resource_configmap "organization-units" "$RESOURCES_BASE_DIR/organization_units"
    create_resource_configmap "user-schemas" "$RESOURCES_BASE_DIR/user_schemas"
    create_resource_configmap "identity-providers" "$RESOURCES_BASE_DIR/identity_providers"
    
    # 4. Install Thunder with Helm
    log_info "Installing Thunder with Helm..."
    if [ ! -d "$HELM_CHART_DIR" ]; then
        log_error "Helm chart directory not found: $HELM_CHART_DIR"
        exit 1
    fi

    # Generate Helm arguments for resource mounts
    HELM_RESOURCE_ARGS=""
    
    generate_helm_args() {
        local type=$1
        local dir=$2
        local key="deployment.resourceMounts.$type"
        
        if [ -d "$dir" ]; then
            local i=0
            # Use find to handle empty directories gracefully, similar to ls logic
            while IFS= read -r file; do
                if [ -f "$file" ]; then
                    local filename=$(basename "$file")
                    HELM_RESOURCE_ARGS="$HELM_RESOURCE_ARGS --set $key[$i]=$filename"
                    i=$((i+1))
                fi
            done < <(find "$dir" -maxdepth 1 -type f 2>/dev/null)
        fi
    }

    generate_helm_args "applications" "$RESOURCES_BASE_DIR/applications"
    generate_helm_args "flows" "$RESOURCES_BASE_DIR/flows"
    generate_helm_args "organization_units" "$RESOURCES_BASE_DIR/organization_units"
    generate_helm_args "user_schemas" "$RESOURCES_BASE_DIR/user_schemas"
    generate_helm_args "identity_providers" "$RESOURCES_BASE_DIR/identity_providers"
    
    log_info "Generated Helm arguments for resources: $HELM_RESOURCE_ARGS"
    
    helm upgrade --install "$HELM_RELEASE" "$HELM_CHART_DIR" \
        --namespace="$NAMESPACE" \
        --create-namespace \
        --wait \
        --timeout=10m \
        $HELM_RESOURCE_ARGS
    
    log_info "Thunder deployed successfully!"
    
    # 5. Display status
    log_info "Deployment Status:"
    kubectl get pods -n "$NAMESPACE"
    echo ""
    log_info "Services:"
    kubectl get svc -n "$NAMESPACE"
    echo ""
    log_info "ConfigMaps:"
    kubectl get configmap -n "$NAMESPACE"
    
    echo ""
    log_info "=========================================="
    log_info "Thunder is now running!"
    log_info "Namespace: $NAMESPACE"
    log_info "Release: $HELM_RELEASE"
    log_info "=========================================="
    echo ""
    log_warn "Press Ctrl+C to cleanup and stop all resources"
    
    # Keep the script running
    while true; do
        sleep 1
    done
}

# Run deployment
deploy
