package pool

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/oarkflow/smpp-server/pkg/smpp"
)

// Connection represents a pooled SMPP connection
type Connection struct {
	client   *smpp.Client
	lastUsed time.Time
	mu       sync.Mutex
}

// NewConnection creates a new pooled connection
func NewConnection(client *smpp.Client) *Connection {
	return &Connection{
		client:   client,
		lastUsed: time.Now(),
	}
}

// Client returns the underlying SMPP client
func (c *Connection) Client() *smpp.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastUsed = time.Now()
	return c.client
}

// LastUsed returns when the connection was last used
func (c *Connection) LastUsed() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastUsed
}

// Close closes the connection
func (c *Connection) Close() error {
	return c.client.Disconnect(context.Background())
}

// IsConnected checks if the connection is still active
func (c *Connection) IsConnected() bool {
	return c.client.IsConnected()
}

// PoolConfig defines the configuration for a connection pool
type PoolConfig struct {
	MaxConnections     int           // Maximum number of connections
	MaxIdleConnections int           // Maximum number of idle connections
	IdleTimeout        time.Duration // How long to keep idle connections
	ConnectTimeout     time.Duration // Timeout for establishing connections
	TestOnBorrow       bool          // Test connection before borrowing
	TestOnReturn       bool          // Test connection before returning to pool
}

// DefaultPoolConfig returns a default pool configuration
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConnections:     10,
		MaxIdleConnections: 5,
		IdleTimeout:        5 * time.Minute,
		ConnectTimeout:     30 * time.Second,
		TestOnBorrow:       true,
		TestOnReturn:       false,
	}
}

// ConnectionPool manages a pool of SMPP connections
type ConnectionPool struct {
	config    PoolConfig
	factory   ConnectionFactory
	semaphore chan struct{} // Limits concurrent connections
	mu        sync.Mutex
	closed    bool
}

// ConnectionFactory creates new SMPP connections
type ConnectionFactory func() (*smpp.Client, error)

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config PoolConfig, factory ConnectionFactory) *ConnectionPool {
	if config.MaxConnections <= 0 {
		config.MaxConnections = 1
	}

	return &ConnectionPool{
		config:    config,
		factory:   factory,
		semaphore: make(chan struct{}, config.MaxConnections),
	}
}

// Get gets a connection from the pool
func (p *ConnectionPool) Get(ctx context.Context) (*PooledClient, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("pool is closed")
	}
	p.mu.Unlock()

	// Acquire semaphore
	select {
	case p.semaphore <- struct{}{}:
		// Got permission to create a connection
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Create new connection
	conn, err := p.factory()
	if err != nil {
		// Return the semaphore token
		<-p.semaphore
		return nil, err
	}

	pooledConn := NewConnection(conn)
	return &PooledClient{conn: pooledConn, pool: p}, nil
}

// Put returns a connection to the pool
func (p *ConnectionPool) Put(conn *Connection) {
	conn.Close()  // Close the connection since we can't reuse it
	<-p.semaphore // Release semaphore
}

// Close closes the connection pool
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	close(p.semaphore)
	return nil
}

// Stats returns pool statistics
func (p *ConnectionPool) Stats() PoolStats {
	return PoolStats{
		ActiveConnections: len(p.semaphore),
		IdleConnections:   cap(p.semaphore) - len(p.semaphore),
		TotalConnections:  cap(p.semaphore),
	}
}

// PoolStats represents pool statistics
type PoolStats struct {
	ActiveConnections int
	IdleConnections   int
	TotalConnections  int
}

// testConnection tests if a connection is still valid
func (p *ConnectionPool) testConnection(conn *Connection) bool {
	return conn.IsConnected()
}

// PooledClient wraps a connection to return it to the pool when closed
type PooledClient struct {
	conn *Connection
	pool *ConnectionPool
	used bool
}

// Connect implements the Client interface
func (pc *PooledClient) Connect(ctx context.Context) error {
	pc.used = true
	return pc.conn.client.Connect(ctx)
}

// Bind implements the Client interface
func (pc *PooledClient) Bind(ctx context.Context) error {
	pc.used = true
	return pc.conn.client.Bind(ctx)
}

// SendSMS implements the Client interface
func (pc *PooledClient) SendSMS(ctx context.Context, sourceAddr, destAddr, message string) error {
	pc.used = true
	return pc.conn.client.SendSMS(ctx, sourceAddr, destAddr, message)
}

// Unbind implements the Client interface
func (pc *PooledClient) Unbind(ctx context.Context) error {
	pc.used = true
	return pc.conn.client.Unbind(ctx)
}

// Disconnect implements the Client interface
func (pc *PooledClient) Disconnect(ctx context.Context) error {
	err := pc.conn.client.Disconnect(ctx)
	if pc.pool != nil {
		pc.pool.Put(pc.conn)
	}
	return err
}

// IsConnected implements the Client interface
func (pc *PooledClient) IsConnected() bool {
	return pc.conn.client.IsConnected()
}

// OnConnect implements the Client interface
func (pc *PooledClient) OnConnect(callback func()) {
	pc.conn.client.OnConnect(callback)
}

// OnDisconnect implements the Client interface
func (pc *PooledClient) OnDisconnect(callback func(error)) {
	pc.conn.client.OnDisconnect(callback)
}

// OnBind implements the Client interface
func (pc *PooledClient) OnBind(callback func()) {
	pc.conn.client.OnBind(callback)
}

// OnUnbind implements the Client interface
func (pc *PooledClient) OnUnbind(callback func()) {
	pc.conn.client.OnUnbind(callback)
}

// OnSubmitResponse implements the Client interface
func (pc *PooledClient) OnSubmitResponse(callback func(messageID string, err error)) {
	pc.conn.client.OnSubmitResponse(callback)
}

// OnMessage implements the Client interface
func (pc *PooledClient) OnMessage(callback func(*smpp.Message)) {
	pc.conn.client.OnMessage(callback)
}

// OnDeliveryReport implements the Client interface
func (pc *PooledClient) OnDeliveryReport(callback func(*smpp.DeliveryReport)) {
	pc.conn.client.OnDeliveryReport(callback)
}
