package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	databasev1 "postgres-operator/api/v1"
)

// Client represents a PostgreSQL database client
type Client struct {
	db *sql.DB
}

// NewClient creates a new PostgreSQL client
func NewClient(ctx context.Context, k8sClient client.Client, cluster *databasev1.PostgresCluster) (*Client, error) {
	// Get credentials from secret
	secret := &corev1.Secret{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      cluster.Name + "-credentials",
		Namespace: cluster.Namespace,
	}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials secret: %w", err)
	}

	password, exists := secret.Data["postgres-password"]
	if !exists {
		return nil, fmt.Errorf("postgres-password not found in secret")
	}

	// Build connection string
	host := fmt.Sprintf("%s-service.%s.svc.cluster.local", cluster.Name, cluster.Namespace)
	connStr := fmt.Sprintf("host=%s port=5432 user=postgres password=%s dbname=%s sslmode=require connect_timeout=10",
		host, string(password), cluster.Spec.Database.Name)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Client{db: db}, nil
}

// Close closes the database connection
func (c *Client) Close() error {
	return c.db.Close()
}

// CreateOrUpdateUser creates or updates a PostgreSQL user
func (c *Client) CreateOrUpdateUser(ctx context.Context, username, password string, privileges []string) error {
	// Validate input
	if username == "" || password == "" {
		return fmt.Errorf("username and password cannot be empty")
	}

	// Check if user exists
	var exists bool
	err := c.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_user WHERE usename = $1)", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if user exists: %w", err)
	}

	if exists {
		// Update existing user
		query := fmt.Sprintf("ALTER USER %s WITH PASSWORD $1", pq.QuoteIdentifier(username))
		_, err = c.db.ExecContext(ctx, query, password)
		if err != nil {
			return fmt.Errorf("failed to update user password: %w", err)
		}
	} else {
		// Create new user
		query := fmt.Sprintf("CREATE USER %s WITH PASSWORD $1", pq.QuoteIdentifier(username))
		_, err = c.db.ExecContext(ctx, query, password)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
	}

	// Grant privileges
	for _, privilege := range privileges {
		query := fmt.Sprintf("GRANT %s TO %s", privilege, pq.QuoteIdentifier(username))
		_, err = c.db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to grant privilege %s: %w", privilege, err)
		}
	}

	return nil
}

// DropUser drops a PostgreSQL user
func (c *Client) DropUser(ctx context.Context, username string) error {
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	// Don't allow dropping system users
	systemUsers := []string{"postgres", "template0", "template1", "pg_monitor", "pg_read_all_settings", "pg_read_all_stats", "pg_stat_scan_tables", "pg_read_server_files", "pg_write_server_files", "pg_execute_server_program"}
	for _, sysUser := range systemUsers {
		if username == sysUser {
			return fmt.Errorf("cannot drop system user: %s", username)
		}
	}

	// Revoke all privileges first
	queries := []string{
		fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %s", pq.QuoteIdentifier(username)),
		fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %s", pq.QuoteIdentifier(username)),
		fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public FROM %s", pq.QuoteIdentifier(username)),
		fmt.Sprintf("REVOKE ALL PRIVILEGES ON SCHEMA public FROM %s", pq.QuoteIdentifier(username)),
		fmt.Sprintf("REVOKE ALL PRIVILEGES ON DATABASE %s FROM %s", pq.QuoteIdentifier(c.getCurrentDatabase(ctx)), pq.QuoteIdentifier(username)),
	}

	for _, query := range queries {
		_, err := c.db.ExecContext(ctx, query)
		if err != nil {
			// Log warning but continue
			fmt.Printf("Warning: failed to revoke privileges with query '%s': %v\n", query, err)
		}
	}

	// Drop user
	query := fmt.Sprintf("DROP USER IF EXISTS %s", pq.QuoteIdentifier(username))
	_, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop user: %w", err)
	}

	return nil
}

// GrantDatabaseAccess grants access to a database for a user
func (c *Client) GrantDatabaseAccess(ctx context.Context, username, database string) error {
	if username == "" || database == "" {
		return fmt.Errorf("username and database cannot be empty")
	}

	// Check if user exists
	var exists bool
	err := c.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_user WHERE usename = $1)", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if user exists: %w", err)
	}

	if !exists {
		return fmt.Errorf("user %s does not exist", username)
	}

	// Grant connect privilege
	query := fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", pq.QuoteIdentifier(database), pq.QuoteIdentifier(username))
	_, err = c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to grant connect privilege: %w", err)
	}

	// Grant usage on schema
	query = fmt.Sprintf("GRANT USAGE ON SCHEMA public TO %s", pq.QuoteIdentifier(username))
	_, err = c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to grant schema usage: %w", err)
	}

	// Grant table privileges
	query = fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %s", pq.QuoteIdentifier(username))
	_, err = c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to grant table privileges: %w", err)
	}

	// Grant sequence privileges
	query = fmt.Sprintf("GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %s", pq.QuoteIdentifier(username))
	_, err = c.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to grant sequence privileges: %w", err)
	}

	return nil
}

// RevokeDatabaseAccess revokes access to a database for a user
func (c *Client) RevokeDatabaseAccess(ctx context.Context, username, database string) error {
	if username == "" || database == "" {
		return fmt.Errorf("username and database cannot be empty")
	}

	queries := []string{
		fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %s", pq.QuoteIdentifier(username)),
		fmt.Sprintf("REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %s", pq.QuoteIdentifier(username)),
		fmt.Sprintf("REVOKE USAGE ON SCHEMA public FROM %s", pq.QuoteIdentifier(username)),
		fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM %s", pq.QuoteIdentifier(database), pq.QuoteIdentifier(username)),
	}

	for _, query := range queries {
		_, err := c.db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to execute revoke query '%s': %w", query, err)
		}
	}

	return nil
}

// GetDatabaseStats returns basic database statistics
func (c *Client) GetDatabaseStats(ctx context.Context) (*DatabaseStats, error) {
	stats := &DatabaseStats{}

	// Get database size
	err := c.db.QueryRowContext(ctx, "SELECT pg_database_size(current_database())").Scan(&stats.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to get database size: %w", err)
	}

	// Get connection count
	err = c.db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()").Scan(&stats.Connections)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection count: %w", err)
	}

	// Get table count
	err = c.db.QueryRowContext(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&stats.Tables)
	if err != nil {
		return nil, fmt.Errorf("failed to get table count: %w", err)
	}

	return stats, nil
}

// getCurrentDatabase returns the current database name
func (c *Client) getCurrentDatabase(ctx context.Context) string {

	
	var dbName string
	err := c.db.QueryRowContext(ctx, "SELECT current_database()").Scan(&dbName)
	if err != nil {
		return "postgres" // fallback to default
	}
	return dbName
}

// DatabaseStats represents database statistics
type DatabaseStats struct {
	Size        int64 `json:"size"`
	Connections int   `json:"connections"`
	Tables      int   `json:"tables"`
}