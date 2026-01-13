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
CUSTOMER_OU_ID=2a3608a4-e9f7-4645-881f-75dfd1052533

log_info "Default resources from immutable configurations"
log_info "------------------------------------------"
log_info "Customer Organization Unit ID: $CUSTOMER_OU_ID"
echo ""


# ============================================================================
# Create WSO2 Cloud Resource Server
# ============================================================================

log_info "Creating WSO2 Cloud resource server..."

RESPONSE=$(thunder_api_call POST "/resource-servers" "{
  \"name\": \"WSO2 Cloud\",
  \"description\": \"WSO2 Cloud resource server\",
  \"identifier\": \"wso2_cloud\",
  \"ouId\": \"${CUSTOMER_OU_ID}\"
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Resource server created successfully"
    WSO2_CLOUD_RS_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -n "$WSO2_CLOUD_RS_ID" ]]; then
        log_info "WSO2 Cloud resource server ID: $WSO2_CLOUD_RS_ID"
    else
        log_error "Could not extract resource server ID from response"
        exit 1
    fi
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Resource server already exists, retrieving ID..."
    RESPONSE=$(thunder_api_call GET "/resource-servers")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        WSO2_CLOUD_RS_ID=$(echo "$BODY" | sed 's/},{/}\n{/g' | grep '"identifier":"wso2_cloud"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        if [[ -n "$WSO2_CLOUD_RS_ID" ]]; then
            log_success "Found resource server ID: $WSO2_CLOUD_RS_ID"
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
# Create Admin Resource
# ============================================================================

log_info "Creating 'admin' resource..."

RESPONSE=$(thunder_api_call POST "/resource-servers/${WSO2_CLOUD_RS_ID}/resources" '{
  "name": "Admin",
  "description": "Administrative resources",
  "handle": "admin",
  "parent": null
}')

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Admin resource created successfully"
    ADMIN_RESOURCE_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -n "$ADMIN_RESOURCE_ID" ]]; then
        log_info "Admin resource ID: $ADMIN_RESOURCE_ID"
    else
        log_error "Could not extract admin resource ID from response"
        exit 1
    fi
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Admin resource already exists, retrieving ID..."
    RESPONSE=$(thunder_api_call GET "/resource-servers/${WSO2_CLOUD_RS_ID}/resources")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        ADMIN_RESOURCE_ID=$(echo "$BODY" | sed 's/},{/}\n{/g' | grep '"handle":"admin"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        if [[ -n "$ADMIN_RESOURCE_ID" ]]; then
            log_success "Found admin resource ID: $ADMIN_RESOURCE_ID"
        else
            log_error "Could not find admin resource ID in response"
            exit 1
        fi
    else
        log_error "Failed to fetch resources (HTTP $HTTP_CODE)"
        exit 1
    fi
else
    log_error "Failed to create admin resource (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

echo ""

# ============================================================================
# Create Admin Actions
# ============================================================================

log_info "Creating admin actions (org_mgt, user_mgt, billing)..."

# Array of admin actions
declare -a ADMIN_ACTIONS=(
    '{"name":"Organization Management","description":"Manage organizations","handle":"org_mgt"}'
    '{"name":"User Management","description":"Manage users","handle":"user_mgt"}'
    '{"name":"Billing","description":"Manage billing","handle":"billing"}'
)

for ACTION_DATA in "${ADMIN_ACTIONS[@]}"; do
    ACTION_HANDLE=$(echo "$ACTION_DATA" | grep -o '"handle":"[^"]*"' | cut -d'"' -f4)
    
    RESPONSE=$(thunder_api_call POST "/resource-servers/${WSO2_CLOUD_RS_ID}/resources/${ADMIN_RESOURCE_ID}/actions" "$ACTION_DATA")
    HTTP_CODE="${RESPONSE: -3}"
    
    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "Action '$ACTION_HANDLE' created"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "Action '$ACTION_HANDLE' already exists, skipping"
    else
        log_error "Failed to create action '$ACTION_HANDLE' (HTTP $HTTP_CODE)"
        exit 1
    fi
done

echo ""

# ============================================================================
# Create Service Resource
# ============================================================================

log_info "Creating 'service' resource..."

RESPONSE=$(thunder_api_call POST "/resource-servers/${WSO2_CLOUD_RS_ID}/resources" '{
  "name": "Service",
  "description": "Service resources",
  "handle": "service",
  "parent": null
}')

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Service resource created successfully"
    SERVICE_RESOURCE_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -n "$SERVICE_RESOURCE_ID" ]]; then
        log_info "Service resource ID: $SERVICE_RESOURCE_ID"
    else
        log_error "Could not extract service resource ID from response"
        exit 1
    fi
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Service resource already exists, retrieving ID..."
    RESPONSE=$(thunder_api_call GET "/resource-servers/${WSO2_CLOUD_RS_ID}/resources")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"

    if [[ "$HTTP_CODE" == "200" ]]; then
        SERVICE_RESOURCE_ID=$(echo "$BODY" | sed 's/},{/}\n{/g' | grep '"handle":"service"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
        if [[ -n "$SERVICE_RESOURCE_ID" ]]; then
            log_success "Found service resource ID: $SERVICE_RESOURCE_ID"
        else
            log_error "Could not find service resource ID in response"
            exit 1
        fi
    else
        log_error "Failed to fetch resources (HTTP $HTTP_CODE)"
        exit 1
    fi
else
    log_error "Failed to create service resource (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

echo ""

# ============================================================================
# Create Service Actions
# ============================================================================

log_info "Creating service actions (ai_agent, asgardeo, bijira, devant, choreo)..."

# Array of service actions
declare -a SERVICE_ACTIONS=(
    '{"name":"AI Agent","description":"AI Agent service","handle":"ai_agent"}'
    '{"name":"Asgardeo","description":"Asgardeo service","handle":"asgardeo"}'
    '{"name":"Bijira","description":"Bijira service","handle":"bijira"}'
    '{"name":"Devant","description":"Devant service","handle":"devant"}'
    '{"name":"Choreo","description":"Choreo Platform","handle":"choreo"}'
)

for ACTION_DATA in "${SERVICE_ACTIONS[@]}"; do
    ACTION_HANDLE=$(echo "$ACTION_DATA" | grep -o '"handle":"[^"]*"' | cut -d'"' -f4)
    
    RESPONSE=$(thunder_api_call POST "/resource-servers/${WSO2_CLOUD_RS_ID}/resources/${SERVICE_RESOURCE_ID}/actions" "$ACTION_DATA")
    HTTP_CODE="${RESPONSE: -3}"
    
    if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
        log_success "Action '$ACTION_HANDLE' created"
    elif [[ "$HTTP_CODE" == "409" ]]; then
        log_warning "Action '$ACTION_HANDLE' already exists, skipping"
    else
        log_error "Failed to create action '$ACTION_HANDLE' (HTTP $HTTP_CODE)"
        exit 1
    fi
done

echo ""

# ============================================================================
# Create Cloud Users
# ============================================================================

log_info "Creating cloud users..."

# Create Cloud Admin User
RESPONSE=$(thunder_api_call POST "/users" "{
  \"type\": \"Customer\",
  \"organizationUnit\": \"${CUSTOMER_OU_ID}\",
  \"attributes\": {
    \"sub\": \"cloudadmin\",
    \"password\": \"admin\",
    \"username\": \"cloudadmin\",
    \"email\": \"cloudadmin@wso2.com\"
  }
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    CLOUDADMIN_USER_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    log_success "Cloud Admin user created (ID: $CLOUDADMIN_USER_ID)"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Cloud Admin user already exists, retrieving ID..."
    RESPONSE=$(thunder_api_call GET "/users")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"
    CLOUDADMIN_USER_ID=$(echo "$BODY" | grep -o '"username":"cloudadmin"' -B 50 | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -z "$CLOUDADMIN_USER_ID" ]]; then
        CLOUDADMIN_USER_ID=$(echo "$BODY" | sed 's/"users":\[/\n/g' | sed 's/},{/}\n{/g' | grep '"username":"cloudadmin"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    fi
    log_success "Found Cloud Admin user ID: $CLOUDADMIN_USER_ID"
else
    log_error "Failed to create Cloud Admin user (HTTP $HTTP_CODE)"
    exit 1
fi

# Create Cloud Biller User
RESPONSE=$(thunder_api_call POST "/users" "{
  \"type\": \"Customer\",
  \"organizationUnit\": \"${CUSTOMER_OU_ID}\",
  \"attributes\": {
    \"sub\": \"cloudbiller\",
    \"password\": \"admin\",
    \"username\": \"cloudbiller\",
    \"email\": \"cloudbiller@wso2.com\"
  }
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    CLOUDBILLER_USER_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    log_success "Cloud Biller user created (ID: $CLOUDBILLER_USER_ID)"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Cloud Biller user already exists, retrieving ID..."
    RESPONSE=$(thunder_api_call GET "/users")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"
    CLOUDBILLER_USER_ID=$(echo "$BODY" | grep -o '"username":"cloudbiller"' -B 50 | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -z "$CLOUDBILLER_USER_ID" ]]; then
        CLOUDBILLER_USER_ID=$(echo "$BODY" | sed 's/"users":\[/\n/g' | sed 's/},{/}\n{/g' | grep '"username":"cloudbiller"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    fi
    log_success "Found Cloud Biller user ID: $CLOUDBILLER_USER_ID"
else
    log_error "Failed to create Cloud Biller user (HTTP $HTTP_CODE)"
    exit 1
fi

# Create Cloud Developer User
RESPONSE=$(thunder_api_call POST "/users" "{
  \"type\": \"Customer\",
  \"organizationUnit\": \"${CUSTOMER_OU_ID}\",
  \"attributes\": {
    \"sub\": \"clouddev\",
    \"password\": \"admin\",
    \"username\": \"clouddev\",
    \"email\": \"clouddev@wso2.com\"
  }
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    CLOUDDEV_USER_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    log_success "Cloud Developer user created (ID: $CLOUDDEV_USER_ID)"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Cloud Developer user already exists, retrieving ID..."
    RESPONSE=$(thunder_api_call GET "/users")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"
    CLOUDDEV_USER_ID=$(echo "$BODY" | grep -o '"username":"clouddev"' -B 50 | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -z "$CLOUDDEV_USER_ID" ]]; then
        CLOUDDEV_USER_ID=$(echo "$BODY" | sed 's/"users":\[/\n/g' | sed 's/},{/}\n{/g' | grep '"username":"clouddev"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    fi
    log_success "Found Cloud Developer user ID: $CLOUDDEV_USER_ID"
else
    log_error "Failed to create Cloud Developer user (HTTP $HTTP_CODE)"
    exit 1
fi

# Create Cloud App Developer User
RESPONSE=$(thunder_api_call POST "/users" "{
  \"type\": \"Customer\",
  \"organizationUnit\": \"${CUSTOMER_OU_ID}\",
  \"attributes\": {
    \"sub\": \"cloudappdev\",
    \"password\": \"admin\",
    \"username\": \"cloudappdev\",
    \"email\": \"cloudappdev@wso2.com\"
  }
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    CLOUDAPPDEV_USER_ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    log_success "Cloud App Developer user created (ID: $CLOUDAPPDEV_USER_ID)"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Cloud App Developer user already exists, retrieving ID..."
    RESPONSE=$(thunder_api_call GET "/users")
    HTTP_CODE="${RESPONSE: -3}"
    BODY="${RESPONSE%???}"
    CLOUDAPPDEV_USER_ID=$(echo "$BODY" | grep -o '"username":"cloudappdev"' -B 50 | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    if [[ -z "$CLOUDAPPDEV_USER_ID" ]]; then
        CLOUDAPPDEV_USER_ID=$(echo "$BODY" | sed 's/"users":\[/\n/g' | sed 's/},{/}\n{/g' | grep '"username":"cloudappdev"' | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    fi
    log_success "Found Cloud App Developer user ID: $CLOUDAPPDEV_USER_ID"
else
    log_error "Failed to create Cloud App Developer user (HTTP $HTTP_CODE)"
    exit 1
fi

echo ""

# ============================================================================
# Create Cloud Admin Role
# ============================================================================

log_info "Creating Cloud Admin role with all permissions..."

RESPONSE=$(thunder_api_call POST "/roles" "{
  \"name\": \"Cloud Admin\",
  \"description\": \"Full administrative access to WSO2 Cloud\",
  \"ouId\": \"${CUSTOMER_OU_ID}\",
  \"permissions\": [
    {
      \"resourceServerId\": \"${WSO2_CLOUD_RS_ID}\",
      \"permissions\": [\"admin:org_mgt\", \"admin:user_mgt\", \"admin:billing\", \"service:ai_agent\", \"service:asgardeo\", \"service:bijira\", \"service:devant\", \"service:choreo\"]
    }
  ],
  \"assignments\": [
    {
      \"id\": \"${CLOUDADMIN_USER_ID}\",
      \"type\": \"user\"
    }
  ]
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Cloud Admin role created successfully"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Cloud Admin role already exists, skipping"
else
    log_error "Failed to create Cloud Admin role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

echo ""

# ============================================================================
# Create Cloud Biller Role
# ============================================================================

log_info "Creating Cloud Biller role with billing access..."

RESPONSE=$(thunder_api_call POST "/roles" "{
  \"name\": \"Cloud Biller\",
  \"description\": \"Billing management access for WSO2 Cloud\",
  \"ouId\": \"${CUSTOMER_OU_ID}\",
  \"permissions\": [
    {
      \"resourceServerId\": \"${WSO2_CLOUD_RS_ID}\",
      \"permissions\": [\"admin:billing\"]
    }
  ],
  \"assignments\": [
    {
      \"id\": \"${CLOUDBILLER_USER_ID}\",
      \"type\": \"user\"
    }
  ]
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Cloud Biller role created successfully"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Cloud Biller role already exists, skipping"
else
    log_error "Failed to create Cloud Biller role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

echo ""

# ============================================================================
# Create Cloud Developer Role
# ============================================================================

log_info "Creating Cloud Developer role with all service access..."

RESPONSE=$(thunder_api_call POST "/roles" "{
  \"name\": \"Cloud Developer\",
  \"description\": \"Access to all WSO2 Cloud services\",
  \"ouId\": \"${CUSTOMER_OU_ID}\",
  \"permissions\": [
    {
      \"resourceServerId\": \"${WSO2_CLOUD_RS_ID}\",
      \"permissions\": [\"service:ai_agent\", \"service:asgardeo\", \"service:bijira\", \"service:devant\", \"service:choreo\"]
    }
  ],
  \"assignments\": [
    {
      \"id\": \"${CLOUDDEV_USER_ID}\",
      \"type\": \"user\"
    }
  ]
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Cloud Developer role created successfully"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Cloud Developer role already exists, skipping"
else
    log_error "Failed to create Cloud Developer role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

echo ""

# ============================================================================
# Create Cloud App Developer Role
# ============================================================================

log_info "Creating Cloud App Developer role with Asgardeo and Choreo access..."

RESPONSE=$(thunder_api_call POST "/roles" "{
  \"name\": \"Cloud App Developer\",
  \"description\": \"Access to Asgardeo and Choreo services\",
  \"ouId\": \"${CUSTOMER_OU_ID}\",
  \"permissions\": [
    {
      \"resourceServerId\": \"${WSO2_CLOUD_RS_ID}\",
      \"permissions\": [\"service:asgardeo\", \"service:choreo\"]
    }
  ],
  \"assignments\": [
    {
      \"id\": \"${CLOUDAPPDEV_USER_ID}\",
      \"type\": \"user\"
    }
  ]
}")

HTTP_CODE="${RESPONSE: -3}"
BODY="${RESPONSE%???}"

if [[ "$HTTP_CODE" == "201" ]] || [[ "$HTTP_CODE" == "200" ]]; then
    log_success "Cloud App Developer role created successfully"
elif [[ "$HTTP_CODE" == "409" ]]; then
    log_warning "Cloud App Developer role already exists, skipping"
else
    log_error "Failed to create Cloud App Developer role (HTTP $HTTP_CODE)"
    echo "Response: $BODY"
    exit 1
fi

# ============================================================================
# Summary
# ============================================================================

log_success "WSO2 Cloud resources setup completed successfully!"
echo ""
log_info "🏢 Organization setup:"
log_info "   - Organization Unit: Cloud Customer"
log_info "   - User Type: Customer"
echo ""
log_info "📦 Created resources:"
log_info "   - Resource Server: WSO2 Cloud"
log_info "   - Admin Resource: org_mgt, user_mgt, billing"
log_info "   - Service Resource: ai_agent, asgardeo, bijira, devant, choreo"
log_info "   - Application: WSO2 Cloud"
echo ""
log_info "👥 Created users and roles:"
log_info "   - cloudadmin (password: admin) → Cloud Admin role"
log_info "   - cloudbiller (password: admin) → Cloud Biller role"
log_info "   - clouddev (password: admin) → Cloud Developer role"
log_info "   - cloudappdev (password: admin) → Cloud App Developer role"
echo ""
