package main

import "github.com/urfave/cli/v2"

func PrintConfig(c *cli.Context) error {
	global, local, err := loadConfiguration(c)
	if err != nil {
		return err
	}

	cfg := local

	if c.IsSet("global") {
		cfg, err = global.Patch(*local)
		if err != nil {
			return err
		}
	}

	for i := range cfg.Configs {
		if i > 0 {
			println("----------------")
		}
		cfg.Configs[i].Print()
	}

	return nil
}
