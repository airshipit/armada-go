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

	"github.com/go-viper/encoding/ini"
	"github.com/spf13/viper"

	"opendev.org/airship/armada-go/pkg/log"
)

// V is the initialized Viper instance populated from the armada config file.
var V *viper.Viper

// Config holds the information required by armada-go commands
type Config struct{}

// Factory is a function which returns ready to use config object and error (if any)
type Factory func() (*Config, error)

// CreateFactory returns function which creates ready to use Config object
func CreateFactory(armadaConfigPath *string) Factory {
	return func() (*Config, error) {
		err := initConfig()
		if err != nil {
			log.Print("Failed to load or initialize config: ", err)
			return nil, err
		}
		return &Config{}, nil
	}
}

func initConfig() error {
	codecRegistry := viper.NewCodecRegistry()
	if err := codecRegistry.RegisterCodec("ini", ini.Codec{}); err != nil {
		return fmt.Errorf("while registering INI codec: %w", err)
	}

	v := viper.NewWithOptions(viper.WithCodecRegistry(codecRegistry))
	v.SetConfigFile("/etc/armada/armada.conf")
	v.SetConfigType("ini")

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("while reading config: %w", err)
	}

	V = v
	return nil
}
