/*
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// Package oauth provides centralized initialization for all OAuth-related services.
package oauth

import (
	"net/http"

	"github.com/senthalan/thunder/backend/internal/attributecache"
	appkg "github.com/senthalan/thunder/backend/pkg/application"
	authnprovidermgr "github.com/senthalan/thunder/backend/internal/authnprovider/manager"
	"github.com/senthalan/thunder/backend/internal/authz"
	"github.com/senthalan/thunder/backend/internal/entityprovider"
	"github.com/senthalan/thunder/backend/internal/flow/flowexec"
	"github.com/senthalan/thunder/backend/internal/inboundclient"
	"github.com/senthalan/thunder/backend/internal/oauth/jwks"
	"github.com/senthalan/thunder/backend/internal/oauth/oauth2/dcr"
	"github.com/senthalan/thunder/backend/internal/oauth/oauth2/discovery"
	"github.com/senthalan/thunder/backend/internal/oauth/oauth2/granthandlers"
	"github.com/senthalan/thunder/backend/internal/oauth/oauth2/introspect"
	"github.com/senthalan/thunder/backend/internal/oauth/oauth2/par"
	"github.com/senthalan/thunder/backend/internal/oauth/oauth2/token"
	"github.com/senthalan/thunder/backend/internal/oauth/oauth2/tokenservice"
	"github.com/senthalan/thunder/backend/internal/oauth/oauth2/userinfo"
	"github.com/senthalan/thunder/backend/internal/oauth/scope"
	"github.com/senthalan/thunder/backend/internal/ou"
	"github.com/senthalan/thunder/backend/internal/resource"
	"github.com/senthalan/thunder/backend/internal/system/crypto/pki"
	"github.com/senthalan/thunder/backend/internal/system/database/provider"
	syshttp "github.com/senthalan/thunder/backend/internal/system/http"
	i18nmgt "github.com/senthalan/thunder/backend/internal/system/i18n/mgt"
	"github.com/senthalan/thunder/backend/internal/system/jose/jwe"
	"github.com/senthalan/thunder/backend/internal/system/jose/jwt"
	"github.com/senthalan/thunder/backend/internal/system/observability"
)

// Initialize initializes all OAuth-related services and registers their routes.
func Initialize(
	mux *http.ServeMux,
	applicationService appkg.ApplicationServiceInterface,
	inboundClient inboundclient.InboundClientServiceInterface,
	authnProvider authnprovidermgr.AuthnProviderManagerInterface,
	jwtService jwt.JWTServiceInterface,
	jweService jwe.JWEServiceInterface,
	flowExecService flowexec.FlowExecServiceInterface,
	observabilitySvc observability.ObservabilityServiceInterface,
	pkiService pki.PKIServiceInterface,
	ouService ou.OrganizationUnitServiceInterface,
	attributeCacheSvc attributecache.AttributeCacheServiceInterface,
	authzService authz.AuthorizationServiceInterface,
	entityProvider entityprovider.EntityProviderInterface,
	resourceService resource.ResourceServiceInterface,
	i18nService i18nmgt.I18nServiceInterface,
) error {
	// Fetch runtime transactioner for OAuth services.
	transactioner, err := provider.GetDBProvider().GetRuntimeDBTransactioner()
	if err != nil {
		return err
	}

	jwks.Initialize(mux, pkiService)
	tokenBuilder, tokenValidator := tokenservice.Initialize(jwtService)
	scopeValidator := scope.Initialize()
	discoveryService := discovery.Initialize(mux, pkiService)
	parService := par.Initialize(mux, inboundClient, authnProvider, jwtService, discoveryService,
		resourceService)
	grantHandlerProvider, err := granthandlers.Initialize(
		mux, jwtService, inboundClient, flowExecService, tokenBuilder, tokenValidator,
		attributeCacheSvc, ouService, authzService, entityProvider, resourceService, parService)
	if err != nil {
		return err
	}
	token.Initialize(mux, jwtService, inboundClient, authnProvider, grantHandlerProvider,
		scopeValidator, observabilitySvc, discoveryService, transactioner)
	introspect.Initialize(mux, jwtService, inboundClient, authnProvider, discoveryService)
	userinfo.Initialize(mux, jwtService, jweService,
		syshttp.NewHTTPClientWithCheckRedirect(func(req *http.Request, _ []*http.Request) error {
			return syshttp.IsSSRFSafeURL(req.URL.String())
		}),
		tokenValidator, inboundClient, ouService, attributeCacheSvc, transactioner)
	dcr.Initialize(mux, applicationService, ouService, i18nService, transactioner)
	return nil
}
