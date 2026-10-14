package fleet_test

import (
	"testing"

	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
)

const testDBPort = 45335

func TestMain(m *testing.M) {
	testpg.Main(m, testDBPort, "fundament-test-pg-platform-admin")
}
