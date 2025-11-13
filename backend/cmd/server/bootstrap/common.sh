#!/bin/bash
# ----------------------------------------------------------------------------
# Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied. See the License for the
# specific language governing permissions and limitations
# under the License.
# ----------------------------------------------------------------------------

# Common functions and variables for bootstrap scripts
# Source this file at the beginning of each bootstrap script

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Logging Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} ✓ $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} ⚠ $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} ✗ $1"
}

log_debug() {
    if [ "${DEBUG:-false}" = "true" ]; then
        echo -e "${CYAN}[DEBUG]${NC} $1"
    fi
}

# API Call Helper Function
thunder_api_call() {
    local method="$1"
    local endpoint="$2"
    local data="${3:-}"

    local url="${THUNDER_API_BASE}${endpoint}"

    log_debug "API Call: $method $url"

    if [ -z "$data" ]; then
        curl -k -s -w "\n%{http_code}" -X "$method" \
            "$url" \
            -H "Content-Type: application/json" 2>/dev/null || echo "000"
    else
        curl -k -s -w "\n%{http_code}" -X "$method" \
            "$url" \
            -H "Content-Type: application/json" \
            -d "$data" 2>/dev/null || echo "000"
    fi
}
