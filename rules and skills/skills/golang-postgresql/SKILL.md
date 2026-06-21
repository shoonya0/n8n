---
name: golang-postgresql
description: "Comprehensive guide for PostgreSQL interaction in Golang. Covers GORM, pgx, connection pooling, and best practices for Postgres-specific features. Use this skill whenever setting up or interacting with PostgreSQL databases in Go."
user-invocable: true
license: MIT
metadata:
  author: AI
  version: "1.0.0"
allowed-tools: Read Edit Write Glob Grep
---

**Persona:** You are a Go database expert who optimizes PostgreSQL queries, sets up robust connection pooling, and handles data migrations carefully.

# PostgreSQL in Golang Best Practices

## 1. Driver Selection

- **pgx** (`github.com/jackc/pgx/v5`): Use this for high-performance PostgreSQL interaction. It supports PostgreSQL-specific features like LISTEN/NOTIFY and array types better than `database/sql`.
- **GORM** (`gorm.io/gorm`): Use this if you prefer an ORM for rapid development and abstraction over raw SQL. 
- **database/sql + lib/pq**: Legacy approach. Use `pgx` instead.

## 2. Setup & Connection Pooling

Always configure the connection pool to prevent exhausting database connections.

### Using GORM:
```go
import (
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func InitDB(dsn string) *gorm.DB {
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        panic("failed to connect to database")
    }

    sqlDB, err := db.DB()
    if err != nil {
        panic(err)
    }

    // SetMaxIdleConns sets the maximum number of connections in the idle connection pool.
    sqlDB.SetMaxIdleConns(10)
    // SetMaxOpenConns sets the maximum number of open connections to the database.
    sqlDB.SetMaxOpenConns(100)
    // SetConnMaxLifetime sets the maximum amount of time a connection may be reused.
    sqlDB.SetConnMaxLifetime(time.Hour)

    return db
}
```

### Using PGX:
```go
import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
)

func InitPGX(dsn string) *pgxpool.Pool {
    config, err := pgxpool.ParseConfig(dsn)
    if err != nil {
        panic(err)
    }
    
    // Configure pooling
    config.MaxConns = 50
    config.MinConns = 10
    
    pool, err := pgxpool.NewWithConfig(context.Background(), config)
    if err != nil {
        panic(err)
    }
    return pool
}
```

## 3. Struct Modeling (GORM)

Use proper tags for indexing, uniqueness, and constraints.

```go
type User struct {
    ID        uint           `gorm:"primaryKey"`
    Email     string         `gorm:"uniqueIndex;not null"`
    Role      string         `gorm:"default:'user'"`
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt gorm.DeletedAt `gorm:"index"` // Adds soft delete functionality
}
```

## 4. Querying and Preventing SQL Injection

Never concatenate strings into your SQL queries. Always use parameterized queries.

```go
// GORM
db.Where("email = ?", email).First(&user)

// PGX
err := pool.QueryRow(ctx, "SELECT id, email FROM users WHERE email = $1", email).Scan(&id, &email)
```

## 5. Transactions

Use transactions for operations that must be atomic (e.g., deducting balance and adding to another).

```go
// GORM Transaction
err := db.Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&user1).Error; err != nil {
        return err // Rollback
    }
    if err := tx.Create(&user2).Error; err != nil {
        return err // Rollback
    }
    return nil // Commit
})
```

## 6. PostgreSQL Specific Features (pgx)

If you need JSONB operations, Arrays, or Listen/Notify, `pgx` handles these natively better than standard `database/sql`.

```go
// JSONB insertion with pgx
_, err = pool.Exec(ctx, "INSERT INTO events (payload) VALUES ($1)", map[string]interface{}{
    "action": "click",
    "user_id": 123,
})
```

## Key Checklists
- [ ] Configure connection pooling (`MaxOpenConns`, `MaxIdleConns`).
- [ ] Use parameterized queries (`$1`, `$2` or `?`) to prevent SQL injection.
- [ ] Wrap multi-statement operations in transactions.
- [ ] Handle `gorm.ErrRecordNotFound` or `pgx.ErrNoRows` gracefully.
