/*
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
     https://www.apache.org/licenses/LICENSE-2.0
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"opendev.org/airship/armada-go/pkg/log"
	"opendev.org/airship/armada-go/pkg/util"
)

// Where possible, json tags match the cli argument names.
// Top level config objects and all values required for proper functioning are not "omitempty".
// Any truly optional piece of config is allowed to be omitted.

// Config holds the information required by armada-go commands
// It is somewhat a superset of what a kubeconfig looks like
type Config struct {
	// +optional
	Kind string `json:"kind,omitempty"`

	// loadedConfigPath is the full path to the location of the config
	// file from which this config was loaded
	// +not persisted in file
	loadedConfigPath string
	//fileSystem       kustfs.FileSystem
}

// Factory is a function which returns ready to use config object and error (if any)
type Factory func() (*Config, error)

// CreateFactory returns function which creates ready to use Config object
func CreateFactory(armadaConfigPath *string) Factory {
	return func() (*Config, error) {
		cfg := NewEmptyConfig()

		var acp string
		if armadaConfigPath != nil {
			acp = *armadaConfigPath
		}

		cfg.initConfigPath(acp)
		err := cfg.LoadConfig()
		if err != nil {
			// Should stop armada-go
			log.Print("Failed to load or initialize config: ", err)
			CreateConfig(acp, true)
		}

		return cfg, nil
	}
}

// CreateConfig saves default config to the specified path
func CreateConfig(armadaConfigPath string, overwrite bool) error {
	cfg := NewConfig()
	cfg.initConfigPath(armadaConfigPath)
	return cfg.PersistConfig(overwrite)
}

// initConfigPath - Initializes loadedConfigPath variable for Config object
func (c *Config) initConfigPath(armadaConfigPath string) {
	switch {
	case armadaConfigPath != "":
		// The loadedConfigPath may already have been received as a command line argument
		c.loadedConfigPath = armadaConfigPath
	case os.Getenv("ARMADA_CONFIG") != "":
		// Otherwise, we can check if we got the path via ENVIRONMENT variable
		c.loadedConfigPath = os.Getenv("ARMADA_CONFIG")
	default:
		// Otherwise, we'll try putting it in the home directory
		c.loadedConfigPath = filepath.Join(util.UserHomeDir(), ".armada", "config")
	}
}

func (c *Config) LoadConfig() error {
	// If I can read from the file, load from it
	// throw an error otherwise
	data, err := os.ReadFile(c.loadedConfigPath)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(data, c)
}

// NewEmptyConfig returns an initialized Config object with no default values
func NewEmptyConfig() *Config {
	return &Config{}
}

// NewConfig returns a newly initialized Config object
func NewConfig() *Config {
	return &Config{
		Kind: "kind",
		//fileSystem: kustfs.MakeFsInMemory(),
	}
}

// ErrConfigFileExists is returned when there is an existing file at specified location
type ErrConfigFileExists struct {
	Path string
}

func (e ErrConfigFileExists) Error() string {
	return fmt.Sprintf("could not create default config at %s, file already exists", e.Path)
}

// ToYaml returns a YAML document
// It serializes the given Config object to a valid YAML document
func (c *Config) ToYaml() ([]byte, error) {
	return yaml.Marshal(&c)
}

// PersistConfig updates the airshipctl config file to match
// the current Config object.
// If file did not previously exist, the file will be created.
// The file will be overwritten if overwrite argument set to true
func (c *Config) PersistConfig(overwrite bool) error {
	if _, err := os.Stat(c.loadedConfigPath); err == nil && !overwrite {
		return ErrConfigFileExists{Path: c.loadedConfigPath}
	}

	airshipConfigYaml, err := c.ToYaml()
	if err != nil {
		return err
	}

	// WriteFile doesn't create the directory, create it if needed
	dir := filepath.Dir(c.loadedConfigPath)
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	// Change the permission of directory
	err = os.Chmod(dir, os.FileMode(0755))
	if err != nil {
		return err
	}

	// Write the Airship Config file
	err = os.WriteFile(c.loadedConfigPath, airshipConfigYaml, 0644)
	if err != nil {
		return err
	}

	// Change the permission of config file
	err = os.Chmod(c.loadedConfigPath, os.FileMode(0644))
	if err != nil {
		return err
	}

	return nil
}
