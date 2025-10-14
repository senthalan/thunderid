# GitHub Copilot Custom Instructions

## Project Overview
This repository is a lightweight user and identity management product written in Go. The backend code is located in the `/backend` directory. There is an optional Next.js frontend in `/frontend` and a sample React Vite app in `/samples/apps`.

## General Guidelines
- Follow general coding best practices, design patterns, and security recommendations.
- Ensure all identity-related code aligns with relevant RFC specifications.
- Promote code reusability and define constants where applicable.

## Backend Project Guidelines

### General
- Reuse common utilities from the `internal/system` packages.
- Define interfaces for services to enable dependency injection and testability.

### Package Structure and Organization
- Follow a modular package structure where each domain/feature lives in its own package under `internal/`.
- Follow a flat directory structure within a package. Avoid nested packages unless absolutely necessary for complex domains.
- Each domain package typically contains related components organized by responsibility (not all files are required):
  - `service.go`: Service interface and implementation (business logic layer)
  - `handler.go`: HTTP handlers (presentation layer) - only if the package exposes HTTP endpoints
  - `store.go`: Data access layer (persistence) - only if the package needs database operations
  - `model.go`: Domain models and DTOs - only if the package has domain-specific models
  - `constants.go`: Package-specific constants - create additional constant files (e.g., `errorconstants.go`, `storeconstants.go`) if needed for better organization
    - `errorconstants.go`: Define service and API errors in this file
    - `storeconstants.go`: Define database queries in this file
    - `utils.go`: Define package-specific utility functions in this file
  - `init.go`: Package initialization and route registration - only for packages with HTTP endpoints
- Adjust the file structure based on actual requirements. For example:
  - No HTTP layer? Skip `handler.go` and `init.go`
  - File-based or cache-backed storage? Add additional storage implementation files
  - Complex domain? Use subdirectories for further organize related functionality (e.g., `internal/oauth/oauth2/`, `internal/oauth/jwks/`).

### Package Exports
- Only export the service interface (e.g., `XServiceInterface`) and models that are used in the service interface from a package.
- Keep all internal implementations (service structs, store interfaces, store implementations, handlers) unexported (lowercase).
- Keep internal constants such as database queries, error codes, and other implementation details unexported (private).
- This ensures proper encapsulation and prevents external packages from depending on internal implementation details.
- Example: Export `UserServiceInterface` and `User` model, but keep `userService`, `userStore`, `userHandler`, and internal query constants unexported.

### Logging
- Use the `log` package in `internal/system` for logging.
- Add minimal info logs and ensure server errors are logged for debugging.
- Avoid logging PII. Use `MaskString` from `internal/system/log` to mask sensitive information.
- Add debug logs where necessary, but avoid excessive logging.
- Use `IsDebugEnabled` from `internal/system/log` if excessive handling is done for debugging log construction.

### Database
- Use `DBClient` in `internal/system/database` for database operations.
- Use `DBQuery` from `internal/system/database/model` to define queries with a unique ID. This allows for DB-specific queries where needed.
  - Define each query with a unique identifier for traceability
  - Support database-specific query variations when necessary (e.g., SQLite vs PostgreSQL)

### Store Layer (Data Access)
- Define store interfaces (e.g., `xStoreInterface`) and implementations (e.g., `xStore` struct) in `store.go`.
- Store layer handles all database interactions and should be used by the service layer.
- Use private constructors (e.g., `newXStore()`) to create store instances.
- Store initialization should use `DBProvider` to obtain database client. Individual store methods should use the created client.
- Keep store methods focused on data access operations without business logic.

### HTTP
- Use `HTTPClient` in `internal/system/http` for sending external requests.

### Cache
- Extend `BaseCache` in `internal/system/cache` for caching requirements.

### Config
- Use `ThunderRuntime` in `internal/system/config` to read system configs.

### Server Constants
- Use constants defined in `internal/system/constants` for reusable global values.

### Error Handling
- Use `ServiceError` from `internal/system/error/serviceerror` to return errors from service layer.
- Use `ErrorResponse` from `internal/system/error/apierror` to define and return API layer errors.
- Avoid logging the same error twice. Return a Go error or `ServiceError` from internal components and log at the service layer.
- Avoid returning unnecessary details from the API layer for server-side errors. Log and return a generic message like "Internal server error" or "Something went wrong" where applicable.

### Defining APIs
- Return JSON responses from APIs where applicable.
- Return JSON errors as per the server `ErrorResponse` definition. For 500 internal server errors, a generic message may be returned.
- Define API handlers in a `handler.go` file within the domain package.
- For packages with HTTP endpoints, use an `init.go` file to register routes with the mux and initialize dependencies.
- Define CORS policies using `middleware.WithCORS` from `internal/system/middleware` where applicable.

### Service Layer and Dependency Injection
- Define service interfaces (e.g., `XServiceInterface`) and implementations (e.g., `xService` struct) in `service.go`.
- Use private constructor functions (e.g., `newXService()`) to create service instances.
- Constructor functions should accept all dependencies as parameters when the service needs external dependencies.
  - Example without dependencies: `func newIDPService() IDPServiceInterface`
  - Example with dependencies: `func newGroupService(ouService OrganizationUnitServiceInterface) GroupServiceInterface`
- Services should depend on interfaces, not concrete implementations, to enable testing with mocks.
- Keep constructors private (unexported) - external packages should only interact through the `Initialize()` function.

### Service Initialization and Dependency Management
- Service initialization should happen **once** during application startup in the `init.go` file of each package.
- The `Initialize(mux, deps)` function in `init.go` should:
  1. Create service instances using constructor functions
  2. Pass initialized service instances as dependencies to other services that need them
  3. Create handlers and inject the service instance into them
  4. Register routes with the mux
  5. Return the service interface for use by dependent packages
- Keep all initialized service instances and pass them to dependent services during their initialization.
- Example initialization flow:
  ```go
  // In internal/group/init.go
  func Initialize(mux *http.ServeMux, ouService ou.OrganizationUnitServiceInterface) GroupServiceInterface {
      groupService := newGroupService(ouService) // Inject dependency via private constructor
      groupHandler := newGroupHandler(groupService)
      registerRoutes(mux, groupHandler)
      return groupService
  }
  ```
- The main service manager should orchestrate all initializations in the correct order, passing dependencies as needed.

### Testing
- Ensure unit tests are written to achieve at least 80% coverage.
- Write integration tests in the `/tests/integration/` directory where applicable.
- Use `stretchr/testify` for tests and follow the test suite pattern.
- `mockery` is used to generate mocks; configurations are in `/backend/.mockery.yml`.

### Documentation
- Add or update documentation in the `README` file or `/docs/content/` for new features or API changes.
- Add Swagger definitions for each new API to `/docs/apis/`.
