package cli

import "fmt"

// version is set at build time via:
//
//	-ldflags "-X github.com/fundament-oss/fundament/functl/pkg/cli.version=<version>"
var version = "dev"

// VersionCmd prints the functl build version.
type VersionCmd struct{}

// Run executes the version command.
func (c *VersionCmd) Run(ctx *Context) error {
	fmt.Println(version)
	return nil
}
