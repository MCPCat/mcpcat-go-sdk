package diagnostics

import (
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
)

func TestInit_EnabledByDefault(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "")
	ResetForTest()
	defer ResetForTest()

	Init("proj_x", false, "officialsdk", "github.com/modelcontextprotocol/go-sdk")
	if !Enabled() {
		t.Fatal("diagnostics must be enabled by default")
	}
}

func TestInit_DisabledViaOption(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "")
	ResetForTest()
	defer ResetForTest()

	Init("proj_x", true, "officialsdk", "p")
	if Enabled() {
		t.Fatal("disabled=true must disable diagnostics")
	}
}

func TestInit_EnvDisableValues(t *testing.T) {
	disable := []string{"true", "TRUE", "1", "yes", "on"}
	for _, v := range disable {
		t.Run("disable_"+v, func(t *testing.T) {
			t.Setenv("DISABLE_DIAGNOSTICS", v)
			ResetForTest()
			defer ResetForTest()
			Init("p", false, "x", "y")
			if Enabled() {
				t.Errorf("%q must disable diagnostics", v)
			}
		})
	}

	stay := []string{"false", "0", "no", "off", "  "}
	for _, v := range stay {
		t.Run("enabled_"+v, func(t *testing.T) {
			t.Setenv("DISABLE_DIAGNOSTICS", v)
			ResetForTest()
			defer ResetForTest()
			Init("p", false, "x", "y")
			if !Enabled() {
				t.Errorf("%q must NOT disable diagnostics", v)
			}
		})
	}
}

func TestInit_RegistersSink(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "")
	ResetForTest()
	defer ResetForTest()

	Init("p", false, "x", "y")
	// The sink should be registered; capturing an Info entry must enqueue a record.
	logging.New().Info("a setup line")
	if n := bufferLenForTest(); n == 0 {
		t.Fatal("Init must register the sink so Info entries are captured")
	}
}
