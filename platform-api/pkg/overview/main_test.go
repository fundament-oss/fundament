package overview_test

import (
	"testing"

	"github.com/fundament-oss/fundament/platform-api/internal/testpg"
)

const testDBPort = 45334

func TestMain(m *testing.M) {
	testpg.Main(m, testDBPort, "fundament-test-pg-platform-public")
}
