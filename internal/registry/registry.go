package registry

import (
	"reflect"
	"sync"

	"github.com/mcpcat/mcpcat-go-sdk/internal/core"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
)

// Global registry for server->MCPcat mapping
var (
	serverMCPcatMap = make(map[uintptr]*core.MCPcatInstance)
	registryMu      sync.RWMutex
)

// Register stores the MCPcat instance for a given server
func Register(server any, instance *core.MCPcatInstance) {
	// Use reflection to get the actual pointer value
	ptr := getPointerValue(server)
	logger := logging.New()
	logger.Debugf("Registry: Registering server %T at pointer 0x%x", server, ptr)

	registryMu.Lock()
	defer registryMu.Unlock()
	serverMCPcatMap[ptr] = instance
	logger.Debugf("Registry: Map now contains %d entries", len(serverMCPcatMap))
}

// Get retrieves the MCPcat instance for a given server
func Get(server any) *core.MCPcatInstance {
	// Use reflection to get the actual pointer value
	ptr := getPointerValue(server)
	logger := logging.New()
	logger.Debugf("Registry: Looking up server %T at pointer 0x%x", server, ptr)

	registryMu.RLock()
	defer registryMu.RUnlock()

	instance := serverMCPcatMap[ptr]
	if instance == nil {
		logger.Debugf("Registry: No instance found. Map contains %d entries:", len(serverMCPcatMap))
		for p := range serverMCPcatMap {
			logger.Debugf("  - Registered pointer: 0x%x", p)
		}
	} else {
		logger.Debugf("Registry: Found instance for pointer 0x%x", ptr)
	}
	return instance
}

// Unregister removes a server from the registry
func Unregister(server any) {
	// Use reflection to get the actual pointer value
	ptr := getPointerValue(server)
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(serverMCPcatMap, ptr)
}

// getPointerValue extracts the actual pointer value from an interface using reflection
func getPointerValue(server any) uintptr {
	if server == nil {
		return 0
	}
	v := reflect.ValueOf(server)
	if v.Kind() == reflect.Ptr {
		return v.Pointer()
	}
	return 0
}
