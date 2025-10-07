package main

import (
	"fmt"

	"github.com/oarkflow/smpp-server/internal/pool"
	"github.com/oarkflow/smpp-server/pkg/smpp"
)

func main() {
	fmt.Println("Testing connection pool...")

	// Create a mock factory function for testing
	factory := func() (*smpp.Client, error) {
		// Mock factory - just testing pool structure
		return nil, fmt.Errorf("mock factory - pool structure test only")
	}

	// Create pool with default config
	config := pool.DefaultPoolConfig()
	config.MaxConnections = 5
	testPool := pool.NewConnectionPool(config, factory)

	// Test pool stats
	stats := testPool.Stats()
	fmt.Printf("✅ Pool created - Max: %d, Active: %d, Idle: %d\n",
		stats.TotalConnections, stats.ActiveConnections, stats.IdleConnections)
}
