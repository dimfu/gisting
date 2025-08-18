package main

import (
	"encoding/json"
	"os"
	"path"

	"golang.org/x/oauth2"
)

// initialize config folder to store the database and the app config itself
func setup(token *oauth2.Token) error {
	var err error
	cfgPath, err = initConfig(token)
	if err != nil {
		return err
	}

	if err := storage.init(cfgPath); err != nil {
		return err
	}

	// handle drop database on binary start up
	if *drop {
		for _, c := range collections {
			if err := storage.db.DropCollection(string(c)); err != nil {
				panic(err)
			}
		}
		// close the previous database to avoid using the previous database lock
		storage.db.Close()
		// and re init the database again
		if err := storage.init(cfgPath); err != nil {
			return err
		}
	}
	return nil
}

func initConfig(token *oauth2.Token) (string, error) {
	userCfgPath, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	configDir := path.Join(userCfgPath, "gisting")

	// ensure root config directory exist
	if _, err := os.Stat(configDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(configDir, 0755); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	// and so does the config file itself
	configFile := path.Join(configDir, "config.json")
	if _, err := os.Stat(configFile); err != nil {
		if os.IsNotExist(err) {
			configFileHandle, err := os.Create(configFile)
			if err != nil {
				return "", err
			}
			defer configFileHandle.Close()

			encoder := json.NewEncoder(configFileHandle)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(&token); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	configFileHandle, err := os.Open(configFile)
	if err != nil {
		return "", err
	}
	defer configFileHandle.Close()

	// NOTE: Currently using token result from the oauth callback as the so called config which is stupid
	decoder := json.NewDecoder(configFileHandle)
	if err := decoder.Decode(&token); err != nil {
		return "", err
	}

	return configDir, nil
}
