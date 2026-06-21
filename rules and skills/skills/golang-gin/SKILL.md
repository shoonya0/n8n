---
name: golang-gin
description: "Comprehensive guide for using the Gin web framework in Golang. Covers routing, middleware, JSON binding, validation, error handling, and file uploads. Use this skill whenever writing or debugging code that uses github.com/gin-gonic/gin."
user-invocable: true
license: MIT
metadata:
  author: AI
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep
---

**Persona:** You are a Go backend engineer who builds fast, robust, and secure web APIs using the Gin framework. You prefer explicit routing, clean middleware, and robust request validation.

# Gin Web Framework Best Practices

Gin is a high-performance HTTP web framework written in Go.

## 1. Project Structure & Routing

Organize your routes logically, often grouping them by version and resource:

```go
func SetupRouter() *gin.Engine {
    // Use New() instead of Default() to explicitly define middleware
    r := gin.New()
    
    // Add standard middleware
    r.Use(gin.Recovery())
    r.Use(gin.Logger())

    // API versioning
    v1 := r.Group("/api/v1")
    {
        users := v1.Group("/users")
        {
            users.GET("", GetUsers)
            users.POST("", CreateUser)
            users.GET("/:id", GetUserByID)
        }
    }
    
    return r
}
```

## 2. Request Binding & Validation

Use `ShouldBindJSON` instead of `BindJSON` to handle errors manually rather than Gin automatically writing a 400 response. Use tags for validation.

```go
type CreateUserRequest struct {
    Email    string `json:"email" binding:"required,email"`
    Password string `json:"password" binding:"required,min=8"`
    Age      int    `json:"age" binding:"omitempty,gte=18"`
}

func CreateUser(c *gin.Context) {
    var req CreateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed: " + err.Error()})
        return
    }

    // Process request
    c.JSON(http.StatusCreated, gin.H{"message": "User created", "email": req.Email})
}
```

## 3. Custom Middleware

Write middleware for authentication, CORS, rate limiting, etc.

```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if token == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            return // Must return after Abort
        }
        
        // Example: Set user ID in context
        c.Set("userID", "12345")
        
        // Pass to next handler
        c.Next()
        
        // Code here executes after the request is processed
    }
}
```

## 4. Error Handling & Responses

Create a standard format for API responses to maintain consistency.

```go
// Good pattern
func RespondWithError(c *gin.Context, code int, message string) {
    c.AbortWithStatusJSON(code, gin.H{"error": message})
}

// Handling server errors
func HandleData(c *gin.Context) {
    data, err := FetchData()
    if err != nil {
        // Log the internal error, but return a generic message to the client
        log.Printf("Internal error: %v", err)
        RespondWithError(c, http.StatusInternalServerError, "An unexpected error occurred")
        return
    }
    c.JSON(http.StatusOK, data)
}
```

## 5. Graceful Shutdown

Always implement a graceful shutdown to ensure requests finish processing before the server stops.

```go
srv := &http.Server{
    Addr:    ":8080",
    Handler: r,
}

go func() {
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("listen: %s\n", err)
    }
}()

// Wait for interrupt signal
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit

log.Println("Shutting down server...")
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := srv.Shutdown(ctx); err != nil {
    log.Fatal("Server forced to shutdown:", err)
}
log.Println("Server exiting")
```

## Key Checklists
- [ ] Use `ShouldBind` instead of `Bind` to handle validation errors gracefully.
- [ ] Group routes logically using `r.Group()`.
- [ ] Use `c.AbortWithStatusJSON()` inside middleware to stop request execution.
- [ ] Never panic in a handler; rely on `gin.Recovery()` middleware for safety.
