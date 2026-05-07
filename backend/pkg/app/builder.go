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

// Package app provides the application builder (orchestrator) for ThunderID.
//
// External consumers can construct a fully wired ThunderID application by injecting
// custom implementations of the core extension-point interfaces:
//
//	app, err := appbuilder.New().
//	    WithServerHome("/opt/thunder").
//	    WithEntityProvider(myEntityProvider).
//	    WithAuthnProvider(myAuthnProvider).
//	    WithConsentService(myConsentService).
//	    WithExecutor("my_executor", myExecutor).
//	    Build(nil)
//
//	if err != nil { ... }
//	defer app.Shutdown()
//	app.Serve()
//
// Dependencies not injected fall back to ThunderID's built-in defaults.
package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/senthalan/thunder/backend/internal/agent"
	"github.com/senthalan/thunder/backend/internal/attributecache"
	"github.com/senthalan/thunder/backend/internal/authn"
	authnAssert "github.com/senthalan/thunder/backend/internal/authn/assert"
	authncm "github.com/senthalan/thunder/backend/internal/authn/common"
	authnConsent "github.com/senthalan/thunder/backend/internal/authn/consent"
	"github.com/senthalan/thunder/backend/internal/authn/github"
	"github.com/senthalan/thunder/backend/internal/authn/google"
	authnOAuth "github.com/senthalan/thunder/backend/internal/authn/oauth"
	authnOIDC "github.com/senthalan/thunder/backend/internal/authn/oidc"
	"github.com/senthalan/thunder/backend/internal/authn/otp"
	"github.com/senthalan/thunder/backend/internal/authn/passkey"
	authnprovidermgr "github.com/senthalan/thunder/backend/internal/authnprovider/manager"
	"github.com/senthalan/thunder/backend/internal/authz"
	"github.com/senthalan/thunder/backend/internal/cert"
	"github.com/senthalan/thunder/backend/internal/consent"
	layoutmgt "github.com/senthalan/thunder/backend/internal/design/layout/mgt"
	"github.com/senthalan/thunder/backend/internal/design/resolve"
	thememgt "github.com/senthalan/thunder/backend/internal/design/theme/mgt"
	"github.com/senthalan/thunder/backend/internal/entity"
	"github.com/senthalan/thunder/backend/internal/entityprovider"
	"github.com/senthalan/thunder/backend/internal/entitytype"
	"github.com/senthalan/thunder/backend/internal/group"
	"github.com/senthalan/thunder/backend/internal/idp"
	"github.com/senthalan/thunder/backend/internal/notification"
	"github.com/senthalan/thunder/backend/internal/ou"
	"github.com/senthalan/thunder/backend/internal/resource"
	"github.com/senthalan/thunder/backend/internal/role"
	"github.com/senthalan/thunder/backend/internal/application"
	flowcore "github.com/senthalan/thunder/backend/internal/flow/core"
	"github.com/senthalan/thunder/backend/internal/flow/executor"
	"github.com/senthalan/thunder/backend/internal/flow/flowexec"
	"github.com/senthalan/thunder/backend/internal/flow/flowmeta"
	flowmgt "github.com/senthalan/thunder/backend/internal/flow/mgt"
	"github.com/senthalan/thunder/backend/internal/inboundclient"
	"github.com/senthalan/thunder/backend/internal/oauth"
	"github.com/senthalan/thunder/backend/internal/system/cache"
	"github.com/senthalan/thunder/backend/internal/system/config"
	"github.com/senthalan/thunder/backend/internal/system/cors"
	"github.com/senthalan/thunder/backend/internal/system/crypto/hash"
	"github.com/senthalan/thunder/backend/internal/system/crypto/pki"
	dbprovider "github.com/senthalan/thunder/backend/internal/system/database/provider"
	declarativeresource "github.com/senthalan/thunder/backend/internal/system/declarative_resource"
	"github.com/senthalan/thunder/backend/internal/system/email"
	"github.com/senthalan/thunder/backend/internal/system/export"
	healthcheckservice "github.com/senthalan/thunder/backend/internal/system/healthcheck/service"
	i18nmgt "github.com/senthalan/thunder/backend/internal/system/i18n/mgt"
	"github.com/senthalan/thunder/backend/internal/system/importer"
	"github.com/senthalan/thunder/backend/internal/system/jose"
	syslog "github.com/senthalan/thunder/backend/internal/system/log"
	"github.com/senthalan/thunder/backend/internal/system/mcp"
	"github.com/senthalan/thunder/backend/internal/system/middleware"
	"github.com/senthalan/thunder/backend/internal/system/observability"
	"github.com/senthalan/thunder/backend/internal/system/security"
	"github.com/senthalan/thunder/backend/internal/system/services"
	"github.com/senthalan/thunder/backend/internal/system/sysauthz"
	"github.com/senthalan/thunder/backend/internal/system/template"
	"github.com/senthalan/thunder/backend/internal/user"
)

const shutdownTimeout = 5 * time.Second

// ThunderApp is the wired application returned by Build. It exposes the HTTP mux
// with all routes registered and provides lifecycle management.
type ThunderApp struct {
	mux                *http.ServeMux
	securityMiddleware func(http.Handler) http.Handler
	observabilitySvc   observability.ObservabilityServiceInterface
	cfg                *config.Config
	serverHome         string
}

// Mux returns the HTTP multiplexer with all application routes registered.
func (a *ThunderApp) Mux() *http.ServeMux { return a.mux }

// SecurityMiddleware returns the pre-wired security middleware. Callers should wrap their
// handler with it: securityMiddleware(myHandler).
func (a *ThunderApp) SecurityMiddleware() func(http.Handler) http.Handler {
	return a.securityMiddleware
}

// Shutdown tears down long-lived background services (observability, etc.).
func (a *ThunderApp) Shutdown() {
	if a.observabilitySvc != nil {
		a.observabilitySvc.Shutdown()
	}
}

// Serve starts the HTTP(S) server and blocks until an OS interrupt or SIGTERM is received,
// then performs a graceful shutdown. WithServerHome must have been called on the Builder
// before Build so that server configuration (address, TLS, etc.) is available.
func (a *ThunderApp) Serve() error {
	if a.cfg == nil {
		return fmt.Errorf("Serve requires server home: call WithServerHome before Build")
	}

	startedAt := time.Now()
	logger := syslog.GetLogger()

	handler := syslog.AccessLogHandler(logger, a.securityMiddleware(a.mux))
	handler = middleware.CorrelationIDMiddleware(handler)

	serverAddr := fmt.Sprintf("%s:%d", a.cfg.Server.Hostname, a.cfg.Server.Port)
	server := &http.Server{
		Addr:              serverAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	var ln net.Listener
	if a.cfg.Server.HTTPOnly {
		l, err := net.Listen("tcp", server.Addr)
		if err != nil {
			return fmt.Errorf("start listener: %w", err)
		}
		ln = l
	} else {
		certFilePath := path.Join(a.serverHome, a.cfg.TLS.CertFile)
		keyFilePath := path.Join(a.serverHome, a.cfg.TLS.KeyFile)
		tlsCfg, err := pki.LoadTLSConfig(a.cfg, certFilePath, keyFilePath)
		if err != nil {
			return fmt.Errorf("load TLS config: %w", err)
		}
		l, err := tls.Listen("tcp", server.Addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("start TLS listener: %w", err)
		}
		ln = l
	}

	a.registerStaticFiles(logger)

	serverURL := config.GetServerURL(&a.cfg.Server)
	logger.Info("ThunderID Server URL", syslog.String("url", serverURL))
	logger.Info("ThunderID Console URL",
		syslog.String("url", fmt.Sprintf("%s/console", strings.TrimSuffix(serverURL, "/"))))

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	srvErr := make(chan error, 1)
	go func() {
		logger.Info("ThunderID Server started",
			syslog.String("startup_time", time.Since(startedAt).String()))
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		}
	}()

	select {
	case err := <-srvErr:
		return err
	case <-sigChan:
		logger.Info("Shutting down server...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("HTTP server shutdown error", syslog.Error(err))
	}

	a.Shutdown()

	if closer := dbprovider.GetDBProviderCloser(); closer != nil {
		if err := closer.Close(); err != nil {
			logger.Error("DB close error", syslog.Error(err))
		}
	}

	if cm := cache.GetCacheManager(); cm != nil {
		cm.Close()
	}

	return nil
}

// registerStaticFiles mounts gate and console frontend apps if their directories exist.
func (a *ThunderApp) registerStaticFiles(logger *syslog.Logger) {
	for _, entry := range []struct{ route, dir string }{
		{"/gate/", path.Join(a.serverHome, "apps", "gate")},
		{"/console/", path.Join(a.serverHome, "apps", "console")},
	} {
		info, err := os.Stat(entry.dir)
		if err != nil || !info.IsDir() {
			logger.Warn("Static app directory not found, skipping",
				syslog.String("directory", entry.dir))
			continue
		}
		a.mux.Handle(entry.route, spaHandler(entry.route, entry.dir))
	}
}

// spaHandler serves a directory as a single-page application with index.html fallback.
func spaHandler(prefix, dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.StripPrefix(prefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || r.URL.Path == "/" {
			serveIndexHTML(w, r, dir)
			return
		}
		if _, err := os.Stat(path.Join(dir, r.URL.Path)); os.IsNotExist(err) {
			serveIndexHTML(w, r, dir)
			return
		}
		fs.ServeHTTP(w, r)
	}))
}

func serveIndexHTML(w http.ResponseWriter, r *http.Request, dir string) {
	indexPath := path.Join(dir, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.ServeFile(w, r, indexPath)
	}
}

// Builder constructs a ThunderApp with optional dependency overrides.
type Builder struct {
	entityProvider   entityprovider.EntityProviderInterface
	authnProvider    authnprovidermgr.AuthnProviderManagerInterface
	consentService   consent.ConsentServiceInterface
	extraExecutors   []extraExecutor
	observabilitySvc observability.ObservabilityServiceInterface
	emailClient      email.EmailClientInterface
	serverHome       string
}

type extraExecutor struct {
	name string
	exec flowcore.ExecutorInterface
}

// New returns a Builder with no overrides — all defaults are used.
func New() *Builder {
	return &Builder{}
}

// WithServerHome sets the ThunderID home directory. When set, Build performs full
// bootstrapping (config loading, cache, CORS, security) and Serve becomes available.
func (b *Builder) WithServerHome(home string) *Builder {
	b.serverHome = home
	return b
}

// WithEntityProvider overrides the default entity provider.
func (b *Builder) WithEntityProvider(ep entityprovider.EntityProviderInterface) *Builder {
	b.entityProvider = ep
	return b
}

// WithAuthnProvider overrides the default authentication provider manager.
func (b *Builder) WithAuthnProvider(ap authnprovidermgr.AuthnProviderManagerInterface) *Builder {
	b.authnProvider = ap
	return b
}

// WithConsentService overrides the default consent service.
func (b *Builder) WithConsentService(cs consent.ConsentServiceInterface) *Builder {
	b.consentService = cs
	return b
}

// WithExecutor registers an additional executor into the flow engine's executor registry.
func (b *Builder) WithExecutor(name string, ex flowcore.ExecutorInterface) *Builder {
	b.extraExecutors = append(b.extraExecutors, extraExecutor{name: name, exec: ex})
	return b
}

// WithObservabilityService overrides the default observability service.
func (b *Builder) WithObservabilityService(obs observability.ObservabilityServiceInterface) *Builder {
	b.observabilitySvc = obs
	return b
}

// WithEmailClient overrides the default SMTP email client.
func (b *Builder) WithEmailClient(client email.EmailClientInterface) *Builder {
	b.emailClient = client
	return b
}

// Build wires all ThunderID services and registers HTTP routes on mux.
// If mux is nil, a new http.ServeMux is created.
// If WithServerHome was called, Build also initialises configuration, cache, CORS,
// and security permissions before wiring services.
func (b *Builder) Build(mux *http.ServeMux) (*ThunderApp, error) { //nolint:gocyclo
	if mux == nil {
		mux = http.NewServeMux()
	}

	var cfg *config.Config
	if b.serverHome != "" {
		var err error
		cfg, err = config.LoadConfig(
			path.Join(b.serverHome, "repository/conf/deployment.yaml"),
			path.Join(b.serverHome, "repository/resources/conf/default.json"),
			b.serverHome,
		)
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		if err = config.InitializeServerRuntime(b.serverHome, cfg); err != nil {
			return nil, fmt.Errorf("initialize server runtime: %w", err)
		}
		if err = cors.InitializeMatcher(cfg.CORS.AllowedOrigins); err != nil {
			return nil, fmt.Errorf("initialize CORS: %w", err)
		}
		cm := cache.GetCacheManager()
		if cm == nil {
			return nil, fmt.Errorf("get cache manager: nil")
		}
		cm.Init()
		security.InitSystemPermissions(cfg.Resource.SystemResourceServer.Handle)
	}

	pkiService, err := pki.Initialize()
	if err != nil {
		return nil, err
	}

	jwtService, jweService, err := jose.Initialize(pkiService)
	if err != nil {
		return nil, err
	}

	observabilitySvc := b.observabilitySvc
	if observabilitySvc == nil {
		observabilitySvc = observability.Initialize()
	}

	var exporters []declarativeresource.ResourceExporter

	i18nService, i18nExporter, err := i18nmgt.Initialize(mux)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, i18nExporter)

	ouAuthzService, err := sysauthz.Initialize()
	if err != nil {
		return nil, err
	}

	ouService, ouHierarchyResolver, ouExporter, err := ou.Initialize(mux, ouAuthzService)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, ouExporter)
	ouAuthzService.SetOUHierarchyResolver(ouHierarchyResolver)

	hashService, err := hash.Initialize()
	if err != nil {
		return nil, err
	}

	consentService := b.consentService
	if consentService == nil {
		consentService = consent.Initialize()
	}

	entityTypeService, entityTypeExporter, err := entitytype.Initialize(
		mux, ouService, ouAuthzService, consentService)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, entityTypeExporter)

	entityService, err := entity.Initialize(hashService, entityTypeService, ouService)
	if err != nil {
		return nil, err
	}

	entityProv := b.entityProvider
	if entityProv == nil {
		entityProv = entityprovider.InitializeEntityProvider(entityService)
	}

	userService, ouUserResolver, userExporter, err := user.Initialize(
		mux, entityService, ouService, entityTypeService, ouAuthzService,
	)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, userExporter)

	groupService, ouGroupResolver, err := group.Initialize(
		mux, dbprovider.GetDBProvider(), ouService, entityService, entityTypeService, ouAuthzService,
	)
	if err != nil {
		return nil, err
	}

	ouService.SetOUUserResolver(ouUserResolver)
	ouService.SetOUGroupResolver(ouGroupResolver)

	resourceService, resourceExporter, err := resource.Initialize(mux, ouService)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, resourceExporter)

	roleService, roleExporter, err := role.Initialize(
		mux, entityService, groupService, ouService, resourceService, entityTypeService,
	)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, roleExporter)

	authZService := authz.Initialize(roleService)

	idpService, idpExporter, err := idp.Initialize(mux)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, idpExporter)

	templateService, err := template.Initialize()
	if err != nil {
		return nil, err
	}

	_, otpService, notifSenderSvc, notificationExporter, err := notification.Initialize(
		mux, jwtService, templateService)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, notificationExporter)

	mcpServer := mcp.Initialize(mux, jwtService)

	passkeyService := passkey.Initialize(entityService)
	otpCoreService := otp.Initialize(otpService, entityProv)

	oauthAuthnService := authnOAuth.Initialize(idpService, entityProv)
	oidcAuthnService := authnOIDC.Initialize(oauthAuthnService, jwtService)
	googleAuthnService := google.Initialize(oidcAuthnService, jwtService)
	githubAuthnService := github.Initialize(oauthAuthnService)

	federatedAuths := map[idp.IDPType]authncm.FederatedAuthenticator{
		idp.IDPTypeOAuth:  oauthAuthnService,
		idp.IDPTypeOIDC:   oidcAuthnService,
		idp.IDPTypeGoogle: googleAuthnService,
		idp.IDPTypeGitHub: githubAuthnService,
	}

	authnProv := b.authnProvider
	if authnProv == nil {
		authnProv = authnprovidermgr.InitializeAuthnProviderManager(
			entityService, passkeyService, otpCoreService, federatedAuths)
	}

	authAssertGen := authnAssert.Initialize()
	consentEnforcer := authnConsent.Initialize(consentService, jwtService)

	authn.Initialize(mux, mcpServer, idpService, jwtService, authnProv, authAssertGen, passkeyService,
		otpCoreService, oauthAuthnService, oidcAuthnService, googleAuthnService, githubAuthnService)

	attributeCacheService := attributecache.Initialize()

	flowFactory, graphCache := flowcore.Initialize()

	emailClient := b.emailClient
	if emailClient == nil {
		if emailClient, err = email.Initialize(); err != nil {
			emailClient = nil
		}
	}

	execRegistry := executor.Initialize(flowFactory, ouService, idpService, notifSenderSvc, jwtService,
		authAssertGen, consentEnforcer, authnProv, otpCoreService, passkeyService, authZService,
		entityTypeService, observabilitySvc, groupService, roleService, entityProv, attributeCacheService,
		emailClient, templateService, oauthAuthnService, oidcAuthnService, githubAuthnService, googleAuthnService)

	for _, ex := range b.extraExecutors {
		execRegistry.RegisterExecutor(ex.name, ex.exec)
	}

	flowMgtService, flowMgtExporter, err := flowmgt.Initialize(mux, mcpServer, flowFactory, execRegistry, graphCache)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, flowMgtExporter)

	certservice, err := cert.Initialize(dbprovider.GetDBProvider())
	if err != nil {
		return nil, err
	}

	themeMgtService, themeExporter, err := thememgt.Initialize(mux)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, themeExporter)

	layoutMgtService, layoutExporter, err := layoutmgt.Initialize(mux)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, layoutExporter)

	inboundClientService, err := inboundclient.Initialize(
		certservice, entityProv, themeMgtService, layoutMgtService, flowMgtService,
		entityTypeService, consentService)
	if err != nil {
		return nil, err
	}

	// TODO: Remove entityService dependency after finalizing declarative resource loading pattern
	applicationService, applicationExporter, err := application.Initialize(
		mux, mcpServer, entityProv, entityService, inboundClientService, ouService, i18nService)
	if err != nil {
		return nil, err
	}
	exporters = append(exporters, applicationExporter)

	if _, err := agent.Initialize(mux, entityService, inboundClientService, ouService); err != nil {
		return nil, err
	}

	designResolveService := resolve.Initialize(mux, themeMgtService, layoutMgtService, applicationService)

	_ = flowmeta.Initialize(mux, applicationService, ouService, designResolveService, i18nService)

	_ = export.Initialize(mux, exporters)

	_ = importer.Initialize(
		mux,
		applicationService,
		idpService,
		flowMgtService,
		ouService,
		entityTypeService,
		roleService,
		resourceService,
		themeMgtService,
		layoutMgtService,
		userService,
		i18nService,
	)

	flowExecService, err := flowexec.Initialize(mux, flowMgtService, applicationService, execRegistry,
		observabilitySvc)
	if err != nil {
		return nil, err
	}

	err = oauth.Initialize(mux, applicationService, inboundClientService, authnProv, jwtService, jweService,
		flowExecService, observabilitySvc, pkiService, ouService, attributeCacheService, authZService, entityProv,
		resourceService, i18nService)
	if err != nil {
		return nil, err
	}

	healthSvc := healthcheckservice.Initialize(dbprovider.GetDBProvider(), dbprovider.GetRedisProvider())
	services.NewHealthCheckService(mux, healthSvc)

	secMw, err := security.Initialize(jwtService)
	if err != nil {
		return nil, err
	}

	return &ThunderApp{
		mux:                mux,
		securityMiddleware: secMw,
		observabilitySvc:   observabilitySvc,
		cfg:                cfg,
		serverHome:         b.serverHome,
	}, nil
}
