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

package executor

import (
	"github.com/senthalan/thunder/backend/internal/attributecache"
	"github.com/senthalan/thunder/backend/internal/authn/assert"
	"github.com/senthalan/thunder/backend/internal/authn/consent"
	"github.com/senthalan/thunder/backend/internal/authn/github"
	"github.com/senthalan/thunder/backend/internal/authn/google"
	"github.com/senthalan/thunder/backend/internal/authn/oauth"
	"github.com/senthalan/thunder/backend/internal/authn/oidc"
	"github.com/senthalan/thunder/backend/internal/authn/otp"
	"github.com/senthalan/thunder/backend/internal/authn/passkey"
	authnprovidermgr "github.com/senthalan/thunder/backend/internal/authnprovider/manager"
	"github.com/senthalan/thunder/backend/internal/authz"
	"github.com/senthalan/thunder/backend/internal/entityprovider"
	"github.com/senthalan/thunder/backend/internal/flow/common"
	"github.com/senthalan/thunder/backend/internal/flow/core"
	"github.com/senthalan/thunder/backend/internal/group"
	"github.com/senthalan/thunder/backend/internal/idp"
	"github.com/senthalan/thunder/backend/internal/notification"
	"github.com/senthalan/thunder/backend/internal/ou"
	"github.com/senthalan/thunder/backend/internal/role"
	"github.com/senthalan/thunder/backend/internal/system/email"
	"github.com/senthalan/thunder/backend/internal/system/jose/jwt"
	"github.com/senthalan/thunder/backend/internal/system/observability"
	"github.com/senthalan/thunder/backend/internal/system/template"

	"github.com/senthalan/thunder/backend/internal/entitytype"
)

// Initialize registers available executors and returns the executor registry.
func Initialize(
	flowFactory core.FlowFactoryInterface,
	ouService ou.OrganizationUnitServiceInterface,
	idpService idp.IDPServiceInterface,
	notifSenderSvc notification.NotificationSenderServiceInterface,
	jwtService jwt.JWTServiceInterface,
	authAssertGen assert.AuthAssertGeneratorInterface,
	consentEnforcer consent.ConsentEnforcerServiceInterface,
	authnProvider authnprovidermgr.AuthnProviderManagerInterface,
	otpService otp.OTPAuthnServiceInterface,
	passkeyService passkey.PasskeyServiceInterface,
	authZService authz.AuthorizationServiceInterface,
	entityTypeService entitytype.EntityTypeServiceInterface,
	observabilitySvc observability.ObservabilityServiceInterface,
	groupService group.GroupServiceInterface,
	roleService role.RoleServiceInterface,
	entityProvider entityprovider.EntityProviderInterface,
	attributeCacheSvc attributecache.AttributeCacheServiceInterface,
	emailClient email.EmailClientInterface,
	templateService template.TemplateServiceInterface,
	oauthSvc oauth.OAuthAuthnServiceInterface,
	oidcSvc oidc.OIDCAuthnServiceInterface,
	githubSvc github.GithubOAuthAuthnServiceInterface,
	googleSvc google.GoogleOIDCAuthnServiceInterface,
) ExecutorRegistryInterface {
	reg := newExecutorRegistry()
	reg.RegisterExecutor(ExecutorNameBasicAuth, newBasicAuthExecutor(
		flowFactory, entityProvider, authnProvider))
	reg.RegisterExecutor(ExecutorNameSMSAuth, newSMSOTPAuthExecutor(
		flowFactory, otpService, authnProvider, entityProvider))
	reg.RegisterExecutor(ExecutorNamePasskeyAuth, newPasskeyAuthExecutor(
		flowFactory, passkeyService, authnProvider, entityProvider))

	reg.RegisterExecutor(ExecutorNameOAuth, newOAuthExecutor(
		"", []common.Input{}, []common.Input{}, flowFactory, idpService, entityTypeService,
		oauthSvc, authnProvider, idp.IDPTypeOAuth))
	reg.RegisterExecutor(ExecutorNameOIDCAuth, newOIDCAuthExecutor(
		"", []common.Input{}, []common.Input{}, flowFactory, idpService, entityTypeService,
		oidcSvc, authnProvider, idp.IDPTypeOIDC))
	reg.RegisterExecutor(ExecutorNameGitHubAuth, newGithubOAuthExecutor(
		flowFactory, idpService, entityTypeService, githubSvc, authnProvider))
	reg.RegisterExecutor(ExecutorNameGoogleAuth, newGoogleOIDCAuthExecutor(
		flowFactory, idpService, entityTypeService, googleSvc, authnProvider))

	reg.RegisterExecutor(ExecutorNameProvisioning, newProvisioningExecutor(flowFactory,
		groupService, roleService, entityProvider, entityTypeService))
	reg.RegisterExecutor(ExecutorNameOUCreation, newOUExecutor(flowFactory, ouService))

	reg.RegisterExecutor(ExecutorNameAttributeCollect, newAttributeCollector(flowFactory, entityProvider))
	reg.RegisterExecutor(ExecutorNameAuthAssert, newAuthAssertExecutor(flowFactory, jwtService,
		ouService, authAssertGen, authnProvider, entityProvider,
		attributeCacheSvc, roleService))
	reg.RegisterExecutor(ExecutorNameAuthorization, newAuthorizationExecutor(flowFactory, authZService, entityProvider))
	reg.RegisterExecutor(ExecutorNameHTTPRequest, newHTTPRequestExecutor(flowFactory, ouService))
	reg.RegisterExecutor(ExecutorNameUserTypeResolver, newUserTypeResolver(flowFactory, entityTypeService, ouService))
	reg.RegisterExecutor(ExecutorNameInviteExecutor, newInviteExecutor(flowFactory))
	reg.RegisterExecutor(ExecutorNameEmailExecutor, newEmailExecutor(
		flowFactory, emailClient, templateService, entityProvider))
	reg.RegisterExecutor(ExecutorNameCredentialSetter, newCredentialSetter(flowFactory, entityProvider))
	reg.RegisterExecutor(ExecutorNamePermissionValidator, newPermissionValidator(flowFactory))
	reg.RegisterExecutor(ExecutorNameIdentifying, newIdentifyingExecutor(
		"", []common.Input{{Identifier: userAttributeUsername, Type: "string", Required: true}}, []common.Input{},
		flowFactory, entityProvider))
	reg.RegisterExecutor(ExecutorNameConsent, newConsentExecutor(flowFactory, consentEnforcer))
	reg.RegisterExecutor(ExecutorNameOUResolver, newOUResolverExecutor(flowFactory, ouService))
	reg.RegisterExecutor(ExecutorNameAttributeUniquenessValidator, newAttributeUniquenessValidator(
		flowFactory, entityTypeService, entityProvider))
	reg.RegisterExecutor(ExecutorNameSMSExecutor, newSMSExecutor(flowFactory, notifSenderSvc, templateService))
	reg.RegisterExecutor(ExecutorNameFederatedAuthResolver, newFederatedAuthResolverExecutor(flowFactory))

	return reg
}
