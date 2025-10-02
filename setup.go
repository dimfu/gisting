package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

var (
	err_unauthorized = errors.New("Not authenticated yet, run gisting without sub command first")
)

type config struct {
	AccessToken string `json:"access_token"`
	ConfigPath  string `json:"configPath"`
	Theme       string `json:"theme"`
}

func (c *config) hasAccessToken() bool {
	return cfg.AccessToken != ""
}

func (c *config) set(name string, value any) error {
	v := reflect.ValueOf(c).Elem()
	f := v.FieldByName(name)

	if !f.IsValid() {
		return fmt.Errorf("no such field: %s", name)
	}
	if !f.CanSet() {
		return fmt.Errorf("cannot set field: %s", name)
	}

	val := reflect.ValueOf(value)

	if !val.Type().AssignableTo(f.Type()) {
		if val.Type().ConvertibleTo(f.Type()) {
			val = val.Convert(f.Type())
		} else {
			return fmt.Errorf("cannot assign value of type %s to field %s of type %s",
				val.Type(), name, f.Type())
		}
	}

	f.Set(val)

	cfgPath := path.Join(c.ConfigPath, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		return err
	}
	return nil
}

func (c *config) clearSecrets() error {
	if err := c.set("AccessToken", ""); err != nil {
		return err
	}
	return nil
}

// initialize config folder to store the database and the app config itself
func setup() error {
	var err error
	cfg, err = initConfig()
	if err != nil {
		return err
	}

	if err := storage.init(cfg.ConfigPath); err != nil {
		return err
	}

	return nil
}

func initLogger() (*os.File, error) {
	f, err := os.OpenFile(path.Join(cfg.ConfigPath, "gisting.log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	log.SetOutput(f)
	return f, nil
}

func initConfig() (*config, error) {
	userCfgPath, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	configDir := filepath.Join(userCfgPath, "gisting")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	configFile := filepath.Join(configDir, "config.json")

	var cfg config

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		cfg = config{
			AccessToken: "",
			ConfigPath:  configDir,
			Theme:       "default",
		}

		if err := writeConfig(configFile, &cfg); err != nil {
			return nil, err
		}
	} else {
		f, err := os.Open(configFile)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		if err := json.NewDecoder(f).Decode(&cfg); err != nil {
			return nil, err
		}
	}

	if _, err := themeSelect(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func themeSelect(cfg *config) (*chroma.Style, error) {
	defaultTheme := "nord"
	if len(cfg.Theme) == 0 {
		if err := cfg.set("Theme", "default"); err != nil {
			return nil, err
		}
		cfg.Theme = "default"
	}

	// use nord as the default theme
	if cfg.Theme == "default" {
		cfg.Theme = defaultTheme
	} else {
		style := styles.Get(cfg.Theme)
		// if theme selected not found in chroma style list, use defaultTheme instead
		if style.Name == "swapoff" {
			cfg.Theme = defaultTheme
		}
	}

	style := styles.Get(cfg.Theme)
	return style, nil
}

func writeConfig(path string, cfg *config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cfg)
}
