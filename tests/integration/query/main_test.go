package query

import (
	"os"
	"testing"

	"github.com/canopy-network/canopyx/tests/integration/helpers"
)

func TestMain(m *testing.M) {
	os.Exit(helpers.Run(m))
}
