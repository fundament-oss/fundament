package cli

import (
	"fmt"

	"github.com/fundament-oss/fundament/functl/pkg/config"
)

// ConfigCmd contains configuration introspection subcommands.
type ConfigCmd struct {
	Dir  ConfigDirCmd  `cmd:"" help:"Print the resolved configuration directory path."`
	Path ConfigPathCmd `cmd:"" help:"Print the resolved configuration file path."`
}

// ConfigDirCmd prints the resolved config directory.
type ConfigDirCmd struct{}

// Run executes the config dir command.
func (c *ConfigDirCmd) Run(ctx *Context) error {
	dir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("resolving config directory: %w", err)
	}
	fmt.Println(dir)
	return nil
}

// ConfigPathCmd prints the resolved config file path.
type ConfigPathCmd struct{}

// Run executes the config path command.
func (c *ConfigPathCmd) Run(ctx *Context) error {
	path, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	fmt.Println(path)
	return nil
}
