# Phase 3: API Layer & Routing

## Objective
Expose the backend functionality via REST APIs using the Gin framework. This phase strictly adheres to the constraints mentioned in `gin-api.md`.

## 1. Data Transfer Objects (DTOs)
Isolate the HTTP request payload mapping from domain models. DTOs must use Gin's `binding` and `validate` tags.

**File Location:** `internal/socialpost/dto/post_request.go`
```go
type PostRequest struct {
    Content   string   `json:"content" binding:"required"`
    Platforms []string `json:"platforms" binding:"required"`
    // Add additional metadata specific to Postman configuration
}
```

## 2. API Handlers
The handler is a thin layer solely responsible for HTTP context management, binding JSON, and delegating work to the Service layer.

**File Location:** `internal/socialpost/handler/post_handler.go`
- Parse the request body into `dto.PostRequest`.
- Return errors via `response.BadRequest(c, errs.CodeValidation, err)`.
- Pass the context (`c.Request.Context()`) and DTO to `service.CreatePost()`.
- Return success using `response.OK(c, data)`. 
- **Rule:** Handlers must **never** contain SQL, framework manipulation of services, or business rules.

## 3. Routing
Register the endpoints to the global Gin RouterGroup.

**File Location:** `internal/socialpost/routes/routes.go`
```go
func RegisterRoutes(rg *gin.RouterGroup, h *handler.PostHandler) {
    p := rg.Group("/socialpost")
    p.POST("", h.CreatePost)
}
```
In `cmd/api/main.go`, `RegisterRoutes` will be invoked and attached to `/api/v1`.

## 4. Middleware Alignment
Ensure the `cmd/api/main.go` registers standard middleware before hitting the `socialpost` routes:
- `RequestID`
- `Logger`
- `Recovery`
- `RateLimiter`

## 5. End-to-End Test (Postman)
- Spin up the `cmd/api/main.go` instance locally.
- Test hitting `http://localhost:8080/api/v1/socialpost` via Postman with varied JSON structures.
- Validate that duplicate payloads receive the configured fallback response via `pkg/response`.
