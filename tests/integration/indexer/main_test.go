package indexer

import (
	"os"
	"testing"

	"github.com/canopy-network/canopyx/tests/integration/helpers"
)

func TestMain(m *testing.M) {
	code := helpers.Run(m)
	os.Exit(code)
}
