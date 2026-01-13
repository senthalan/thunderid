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

# Bootstrap Script: Default Resources Setup
# Creates default organization unit, user schema, admin user, system resource server, system action, admin role, and DEVELOP application

set -e

# Source common functions from the same directory as this script
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]:-$0}")"
source "${SCRIPT_DIR}/common.sh"

# ============================================================================
# Log Default Resource Information
# ============================================================================
EMPLOYEE_OU_ID=84f39d88-3db9-4826-a37a-78e21cd1786f

log_info "Default resources from immutable configurations"
log_info "------------------------------------------"
log_info "Employee Organization Unit ID: $EMPLOYEE_OU_ID"
log_info "------------------------------------------"

# ============================================================================
# Log Configuration Files
# ============================================================================

log_info "Deployment configuration:"
log_info "------------------------------------------"
if [[ -f "repository/conf/deployment.yaml" ]]; then
    cat repository/conf/deployment.yaml
else
    log_warning "deployment.yaml not found at repository/conf/deployment.yaml"
fi
log_info "------------------------------------------"

echo ""

log_info "Repository resources structure:"
log_info "------------------------------------------"
if command -v tree &> /dev/null; then
    if [[ -d "repository/resources" ]]; then
        tree repository/resources
    else
        log_warning "repository/resources directory not found"
    fi
else
    log_warning "tree command not available, using ls -R instead"
    if [[ -d "repository/resources" ]]; then
        ls -R repository/resources
    else
        log_warning "repository/resources directory not found"
    fi
fi
log_info "------------------------------------------"

echo ""

# ============================================================================
# Create Admin User
# ============================================================================

log_info "Creating admin user..."

RESPONSE=$(thunder_api_call POST "/users" '{
  "type": "Employee",
  "organizationUnit": "'${EMPLOYEE_OU_ID}'",
  "attributes": {
    "username": "admin",
    "password": "admin",
    "sub": "admin",
    "email": "admin@thunder.dev",
    "email_verified": true,
    "name": "Administrator",
    "given_name": "Admin",
    "family_name": "User",
    "picture": "https://example.com/avatar.jpg",
    "phone_number": "+12345678920",
    "phone_number_verified": true
  }
}')

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Admin user created successfully"
    log_info "Username: admin"
    log_info "Password: admin"

    # Extract admin user ID
    ADMIN_USER_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -z "$ADMIN_USER_ID" ]]; then
        log_warning "Could not extract admin user ID from response"
    else
        log_info "Admin user ID: $ADMIN_USER_ID"
    fi
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Admin user already exists, retrieving user ID..."

    # Get existing admin user ID
    RESPONSE=$(thunder_api_call GET "/users")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        # Parse JSON to find admin user
        ADMIN_USER_ID=$(echo "$BODY" | grep -o '"id":"[^"]*","[^"]*":"[^"]*","attributes":{[^}]*"username":"admin"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

        # Fallback parsing
        if [[ -z "$ADMIN_USER_ID" ]]; then
            ADMIN_USER_ID=$(echo "$BODY" | sed 's/},{/}\n{/g' | grep '"username":"admin"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        fi

        if [[ -n "$ADMIN_USER_ID" ]]; then
            log_success "Found admin user ID: $ADMIN_USER_ID"
        else
            log_error "Could not find admin user in response"
            exit 1
        fi
    else
        log_error "Failed to fetch users (HTTP $HTTP_CODE)"
        exit 1
    fi
else
    log_error "Failed to create admin user (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

echo ""

# ============================================================================
# Create System Resource Server
# ============================================================================

log_info "Creating system resource server..."

RESPONSE=$(thunder_api_call POST "/resource-servers" "{
  \"name\": \"System\",
  \"description\": \"System resource server\",
  \"identifier\": \"system\",
  \"ouId\": \"${EMPLOYEE_OU_ID}\"
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Resource server created successfully"
    SYSTEM_RS_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -n "$SYSTEM_RS_ID" ]]; then
        log_info "System resource server ID: $SYSTEM_RS_ID"
    else
        log_error "Could not extract resource server ID from response"
        exit 1
    fi
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Resource server already exists, retrieving ID..."
    # Get existing resource server ID
    RESPONSE=$(thunder_api_call GET "/resource-servers")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        SYSTEM_RS_ID=$(echo "$BODY" | grep -o '"id":"[^"]*","[^"]*":"System"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

        # Fallback parsing
        if [[ -z "$SYSTEM_RS_ID" ]]; then
            SYSTEM_RS_ID=$(echo "$BODY" | sed 's/},{/}\n{/g' | grep '"identifier":"system"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        fi

        if [[ -n "$SYSTEM_RS_ID" ]]; then
            log_success "Found resource server ID: $SYSTEM_RS_ID"
        else
            log_error "Could not find resource server ID in response"
            exit 1
        fi
    else
        log_error "Failed to fetch resource servers (HTTP $HTTP_CODE)"
        exit 1
    fi
else
    log_error "Failed to create resource server (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

echo ""

# ============================================================================
# Create System Action
# ============================================================================

log_info "Creating 'system' action on resource server..."

if [[ -z "$SYSTEM_RS_ID" ]]; then
    log_error "System resource server ID is not available. Cannot create action."
    exit 1
fi

RESPONSE=$(thunder_api_call POST "/resource-servers/${SYSTEM_RS_ID}/actions" '{
  "name": "System Access",
  "description": "Full system access permission",
  "handle": "system"
}')

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "System action created successfully"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "System action already exists, skipping"
else
    log_error "Failed to create system action (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

echo ""

# ============================================================================
# Create Admin Role
# ============================================================================

log_info "Creating admin role with 'system' permission..."

if [[ -z "$ADMIN_USER_ID" ]]; then
    log_error "Admin user ID is not available. Cannot create role."
    exit 1
fi

if [[ -z "$SYSTEM_RS_ID" ]]; then
    log_error "System resource server ID is not available. Cannot create role."
    exit 1
fi

RESPONSE=$(thunder_api_call POST "/roles" "{
  \"name\": \"Administrator\",
  \"description\": \"System administrator role with full permissions\",
  \"ouId\": \"${EMPLOYEE_OU_ID}\",
  \"permissions\": [
    {
      \"resourceServerId\": \"${SYSTEM_RS_ID}\",
      \"permissions\": [\"system\"]
    }
  ],
  \"assignments\": [
    {
      \"id\": \"${ADMIN_USER_ID}\",
      \"type\": \"user\"
    }
  ]
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Admin role created and assigned to admin user"
    ADMIN_ROLE_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -n "$ADMIN_ROLE_ID" ]]; then
        log_info "Admin role ID: $ADMIN_ROLE_ID"
    fi
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Admin role already exists"
else
    log_error "Failed to create admin role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

echo ""


# ============================================================================
# Summary
# ============================================================================

log_success "Default resources setup completed successfully!"
echo ""
log_info "👤 Admin credentials:"
log_info "   Username: admin"
log_info "   Password: admin"
log_info "   Role: Administrator (system permission)"
echo ""
