package main

import (
	"errors"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"
)

const (
	configFileName = ".goseal"
)

type PathSupplier func() (string, error)

type StageConfiguration struct {
	Name     string `yaml:"name"`
	Cert     string `yaml:"cert"`
	BasePath string `yaml:"basePath"`
}

func (sc StageConfiguration) Print() {
	println("name: ", sc.Name)
	println("cert: ", sc.Cert)
	println("basepath: ", sc.BasePath)
}

type Configuration struct {
	Configs []StageConfiguration `yaml:"configs"`
}

func (c Configuration) Patch(other Configuration) (*Configuration, error) {
	c.Configs = append(c.Configs, other.Configs...)
	return &c, nil
}

func (c Configuration) GetStageConfigByName(name string) (*StageConfiguration, error) {
	for _, v := range c.Configs {
		if v.Name != name {
			continue
		}

		return &v, nil
	}

	return nil, errors.New("not found")
}

func getGlobalConfigPath() (string, error) {
	userHomeDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return userHomeDir + "/goseal", nil
}

func getLocalConfigPath() (string, error) {
	return os.Getwd()
}

func writeConfiguration(c *Configuration, isGlobal bool) error {
	pathSupplier := getLocalConfigPath
	if isGlobal {
		pathSupplier = getGlobalConfigPath
	}

	var out []byte
	var err error
	if out, err = yaml.Marshal(c); err != nil {
		return err
	}

	var path string
	if path, err = pathSupplier(); err != nil {
		return err
	}

	return os.WriteFile(configPath(path), out, os.ModePerm)
}

func configPath(path string) string {
	return path + "/" + configFileName
}

func getStageConfigurationByName(c *cli.Context, configName string) (*StageConfiguration, error) {
	g, l, err := loadConfiguration(c)
	if err != nil {
		return nil, err
	}

	cfg, err := g.Patch(*l)
	if err != nil {
		return nil, err
	}

	return cfg.GetStageConfigByName(configName)
}

func loadConfiguration(c *cli.Context) (*Configuration, *Configuration, error) {
	global, err := load(getGlobalConfigPath)
	if err != nil {
		return nil, nil, err
	}

	local, err := load(getLocalConfigPath)
	if err != nil {
		return nil, nil, err
	}

	return global, local, nil
}

func load(ps PathSupplier) (*Configuration, error) {
	var path string
	var err error

	if path, err = ps(); err != nil {
		return nil, err
	}

	cfg, err := loadConfigFile(path)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadConfigFile(path string) (*Configuration, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}

	configPath := configPath(path)
	info, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		return &Configuration{}, nil
	}

	if info.IsDir() {
		return &Configuration{}, nil
	}

	fileBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	cfg := &Configuration{}
	if err := yaml.Unmarshal(fileBytes, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func ensureDirForFile(file string) error {
	i := strings.LastIndex(file, "/")
	dirPath := file[:i]
	return ensureDir(dirPath)
}

func ensureDir(dirName string) error {
	err := os.MkdirAll(dirName, os.ModePerm)
	if err == nil {
		return nil
	}
	if os.IsExist(err) {
		// check that the existing path is a directory
		info, err := os.Stat(dirName)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("path exists but is not a directory")
		}
		return nil
	}
	return err
}
