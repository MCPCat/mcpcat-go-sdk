package registry

import (
	"sync"
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
)

// mockServer is a simple mock server type for testing
type mockServer struct {
	name string
}

// TestRegister tests the Register function
func TestRegister(t *testing.T) {
	tests := []struct {
		name     string
		server   any
		instance *core.MCPcatInstance
		wantNil  bool
	}{
		{
			name:   "register pointer server",
			server: &mockServer{name: "test1"},
			instance: &core.MCPcatInstance{
				ProjectID: "proj1",
				Options:   &core.Options{},
			},
			wantNil: false,
		},
		{
			name:   "register another pointer server",
			server: &mockServer{name: "test2"},
			instance: &core.MCPcatInstance{
				ProjectID: "proj2",
				Options:   &core.Options{},
			},
			wantNil: false,
		},
		{
			name:   "register nil server",
			server: nil,
			instance: &core.MCPcatInstance{
				ProjectID: "proj3",
				Options:   &core.Options{},
			},
			wantNil: false, // nil and non-pointers both map to pointer value 0 and can be stored
		},
		{
			name:   "register non-pointer type",
			server: mockServer{name: "test3"},
			instance: &core.MCPcatInstance{
				ProjectID: "proj4",
				Options:   &core.Options{},
			},
			wantNil: false, // non-pointers map to pointer value 0 and can be stored
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear the registry before each test
			clearRegistry()

			Register(tt.server, tt.instance)
			got := Get(tt.server)

			if tt.wantNil {
				if got != nil {
					t.Errorf("Get() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Error("Get() = nil, want non-nil")
				} else if got.ProjectID != tt.instance.ProjectID {
					t.Errorf("Get().ProjectID = %v, want %v", got.ProjectID, tt.instance.ProjectID)
				}
			}
		})
	}
}

// TestGet tests the Get function
func TestGet(t *testing.T) {
	tests := []struct {
		name       string
		setupFunc  func() any
		getServer  any
		wantNil    bool
		wantProjID string
	}{
		{
			name: "get registered server",
			setupFunc: func() any {
				server := &mockServer{name: "test1"}
				Register(server, &core.MCPcatInstance{
					ProjectID: "proj1",
					Options:   &core.Options{},
				})
				return server
			},
			wantNil:    false,
			wantProjID: "proj1",
		},
		{
			name: "get unregistered server",
			setupFunc: func() any {
				return &mockServer{name: "unregistered"}
			},
			wantNil: true,
		},
		{
			name: "get nil server",
			setupFunc: func() any {
				return nil
			},
			wantNil: true,
		},
		{
			name: "get non-pointer type",
			setupFunc: func() any {
				return mockServer{name: "nonpointer"}
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear the registry before each test
			clearRegistry()

			server := tt.setupFunc()
			got := Get(server)

			if tt.wantNil {
				if got != nil {
					t.Errorf("Get() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Error("Get() = nil, want non-nil")
				} else if got.ProjectID != tt.wantProjID {
					t.Errorf("Get().ProjectID = %v, want %v", got.ProjectID, tt.wantProjID)
				}
			}
		})
	}
}

// TestUnregister tests the Unregister function
func TestUnregister(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() any
	}{
		{
			name: "unregister existing server",
			setupFunc: func() any {
				server := &mockServer{name: "test1"}
				Register(server, &core.MCPcatInstance{
					ProjectID: "proj1",
					Options:   &core.Options{},
				})
				return server
			},
		},
		{
			name: "unregister non-existent server",
			setupFunc: func() any {
				return &mockServer{name: "nonexistent"}
			},
		},
		{
			name: "unregister nil server",
			setupFunc: func() any {
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear the registry before each test
			clearRegistry()

			server := tt.setupFunc()
			Unregister(server)

			// After unregister, Get should return nil
			got := Get(server)
			if got != nil {
				t.Errorf("Get() after Unregister() = %v, want nil", got)
			}
		})
	}
}

// TestGetPointerValue tests the getPointerValue helper function
func TestGetPointerValue(t *testing.T) {
	server := &mockServer{name: "test"}

	tests := []struct {
		name     string
		input    any
		wantZero bool
	}{
		{
			name:     "pointer type",
			input:    server,
			wantZero: false,
		},
		{
			name:     "nil value",
			input:    nil,
			wantZero: true,
		},
		{
			name:     "non-pointer type",
			input:    mockServer{name: "test"},
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPointerValue(tt.input)

			if tt.wantZero {
				if got != 0 {
					t.Errorf("getPointerValue() = %v, want 0", got)
				}
			} else {
				if got == 0 {
					t.Error("getPointerValue() = 0, want non-zero")
				}
			}
		})
	}
}

// TestConcurrentAccess tests thread-safety of registry operations
func TestConcurrentAccess(t *testing.T) {
	clearRegistry()

	const numGoroutines = 100
	const numOperations = 1000

	var wg sync.WaitGroup
	servers := make([]*mockServer, numGoroutines)

	// Create servers
	for i := 0; i < numGoroutines; i++ {
		servers[i] = &mockServer{name: "test"}
	}

	// Concurrent Register operations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				Register(servers[idx], &core.MCPcatInstance{
					ProjectID: "concurrent",
					Options:   &core.Options{},
				})
			}
		}(i)
	}
	wg.Wait()

	// Concurrent Get operations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				instance := Get(servers[idx])
				if instance == nil {
					t.Errorf("Get() returned nil for server %d", idx)
				}
			}
		}(i)
	}
	wg.Wait()

	// Concurrent Unregister operations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			Unregister(servers[idx])
		}(i)
	}
	wg.Wait()

	// Verify all servers are unregistered
	for i, server := range servers {
		if instance := Get(server); instance != nil {
			t.Errorf("Server %d still registered after Unregister()", i)
		}
	}
}

// TestConcurrentMixedOperations tests concurrent mixed read/write operations
func TestConcurrentMixedOperations(t *testing.T) {
	clearRegistry()

	const numGoroutines = 50
	var wg sync.WaitGroup
	servers := make([]*mockServer, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		servers[i] = &mockServer{name: "test"}
	}

	// Mix of Register, Get, and Unregister operations
	wg.Add(numGoroutines * 3)

	for i := 0; i < numGoroutines; i++ {
		// Register goroutine
		go func(idx int) {
			defer wg.Done()
			Register(servers[idx], &core.MCPcatInstance{
				ProjectID: "mixed",
				Options:   &core.Options{},
			})
		}(i)

		// Get goroutine
		go func(idx int) {
			defer wg.Done()
			Get(servers[idx])
		}(i)

		// Unregister goroutine
		go func(idx int) {
			defer wg.Done()
			Unregister(servers[idx])
		}(i)
	}

	wg.Wait()
}

// TestRegistryLifecycle tests the full lifecycle of register -> get -> unregister
func TestRegistryLifecycle(t *testing.T) {
	clearRegistry()

	server := &mockServer{name: "lifecycle"}
	instance := &core.MCPcatInstance{
		ProjectID: "lifecycle-proj",
		Options:   &core.Options{},
	}

	// Step 1: Initially not registered
	got := Get(server)
	if got != nil {
		t.Errorf("Step 1: Get() = %v, want nil", got)
	}

	// Step 2: Register
	Register(server, instance)
	got = Get(server)
	if got == nil {
		t.Fatal("Step 2: Get() = nil, want non-nil")
	}
	if got.ProjectID != instance.ProjectID {
		t.Errorf("Step 2: ProjectID = %v, want %v", got.ProjectID, instance.ProjectID)
	}

	// Step 3: Unregister
	Unregister(server)
	got = Get(server)
	if got != nil {
		t.Errorf("Step 3: Get() = %v, want nil", got)
	}

	// Step 4: Re-register
	Register(server, instance)
	got = Get(server)
	if got == nil {
		t.Fatal("Step 4: Get() = nil, want non-nil")
	}
}

// TestMultipleServers tests registering multiple different servers
func TestMultipleServers(t *testing.T) {
	clearRegistry()

	servers := []*mockServer{
		{name: "server1"},
		{name: "server2"},
		{name: "server3"},
	}

	instances := []*core.MCPcatInstance{
		{ProjectID: "proj1", Options: &core.Options{}},
		{ProjectID: "proj2", Options: &core.Options{}},
		{ProjectID: "proj3", Options: &core.Options{}},
	}

	// Register all servers
	for i, server := range servers {
		Register(server, instances[i])
	}

	// Verify all servers are registered correctly
	for i, server := range servers {
		got := Get(server)
		if got == nil {
			t.Errorf("Server %d: Get() = nil, want non-nil", i)
			continue
		}
		if got.ProjectID != instances[i].ProjectID {
			t.Errorf("Server %d: ProjectID = %v, want %v", i, got.ProjectID, instances[i].ProjectID)
		}
	}

	// Unregister middle server
	Unregister(servers[1])

	// Verify server 1 is unregistered
	if got := Get(servers[1]); got != nil {
		t.Errorf("Server 1 after unregister: Get() = %v, want nil", got)
	}

	// Verify other servers are still registered
	for _, idx := range []int{0, 2} {
		got := Get(servers[idx])
		if got == nil {
			t.Errorf("Server %d: Get() = nil, want non-nil", idx)
		}
	}
}

// TestRegisterOverwrite tests that re-registering a server overwrites the previous instance
func TestRegisterOverwrite(t *testing.T) {
	clearRegistry()

	server := &mockServer{name: "test"}

	instance1 := &core.MCPcatInstance{
		ProjectID: "proj1",
		Options:   &core.Options{},
	}

	instance2 := &core.MCPcatInstance{
		ProjectID: "proj2",
		Options:   &core.Options{},
	}

	// Register with instance1
	Register(server, instance1)
	got := Get(server)
	if got == nil || got.ProjectID != "proj1" {
		t.Error("First registration failed")
	}

	// Re-register with instance2
	Register(server, instance2)
	got = Get(server)
	if got == nil {
		t.Fatal("Get() = nil after second registration")
	}
	if got.ProjectID != "proj2" {
		t.Errorf("ProjectID = %v, want proj2 (should be overwritten)", got.ProjectID)
	}
}

// clearRegistry is a helper function to clear the registry between tests
func clearRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	serverMCPcatMap = make(map[uintptr]*core.MCPcatInstance)
}
