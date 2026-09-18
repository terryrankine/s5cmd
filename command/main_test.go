package command

import (
	"os"
	"testing"

	"github.com/peak/s5cmd/v2/log"
)

// The commands print through the package-level logger, which main.go
// initialises. Unit tests that reach an error path need it too.
func TestMain(m *testing.M) {
	if err := log.Init("error", false); err != nil {
		panic(err)
	}
	code := m.Run()
	log.Close()
	os.Exit(code)
}
