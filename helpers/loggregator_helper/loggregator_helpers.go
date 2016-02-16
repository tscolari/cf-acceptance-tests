package loggregator_helpers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
	"github.com/cloudfoundry-incubator/cf-test-helpers/helpers"
)

var DEFAULT_TIMEOUT = 30 * time.Second

type CfHomeConfig struct {
	AccessToken         string
	LoggregatorEndpoint string
}

func GetCfHomeConfig() *CfHomeConfig {
	config := helpers.LoadConfig()
	context := helpers.NewContext(config)
	myCfHomeConfig := &CfHomeConfig{}

	cf.AsUser(context.AdminUserContext(), DEFAULT_TIMEOUT, func() {
		path := filepath.Join(os.Getenv("CF_HOME"), ".cf", "config.json")

		configFile, err := os.Open(path)
		if err != nil {
			panic(err)
		}

		decoder := json.NewDecoder(configFile)
		err = decoder.Decode(myCfHomeConfig)
		if err != nil {
			panic(err)
		}
	})

	return myCfHomeConfig
}
