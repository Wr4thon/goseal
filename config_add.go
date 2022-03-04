package main

import (
	"github.com/urfave/cli/v2"
)

func AddConfig(c *cli.Context) error {
	global, cfg, err := loadConfiguration(c)
	if err != nil {
		return err
	}

	if c.IsSet("global") {
		cfg = global
	}

	newConfig := StageConfiguration{}

	cfg.Configs = append(cfg.Configs, newConfig)

	return writeConfiguration(cfg, c.IsSet("global"))
}
