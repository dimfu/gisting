package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"

	"golang.org/x/oauth2"
)

type config struct {
	ClientSecret string       `json:"clientSecret"`
	ClientID     string       `json:"clientId"`
	Token        oauth2.Token `json:"token"`
	ConfigPath   string       `json:"configPath"`
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
	if err := c.set("ClientSecret", ""); err != nil {
		return err
	}
	if err := c.set("ClientID", ""); err != nil {
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

	if err := storage.init(cfg.ConfigPath, *drop); err != nil {
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
			ClientSecret: "",
			ClientID:     "",
			Token:        oauth2.Token{},
			ConfigPath:   configDir,
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

	return &cfg, nil
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
