package mcpcat

import "github.com/mcpcat/mcpcat-go-sdk/internal/core"

type (
	UserIdentity   = core.UserIdentity
	Options        = core.Options
	RedactFunc     = core.RedactFunc
	Exporter       = core.Exporter
	ExporterConfig = core.ExporterConfig
	Event          = core.Event
	MCPcatInstance = core.MCPcatInstance
	Session        = core.Session
)

type IDPrefix = core.IDPrefix

const (
	PrefixSession IDPrefix = core.PrefixSession
	PrefixEvent   IDPrefix = core.PrefixEvent
)

func DefaultOptions() Options {
	return core.DefaultOptions()
}
