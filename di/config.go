package di

import (
	"github.com/goava/di"
	"github.com/spf13/viper"
)

var config = di.Options(
	di.Provide(newConfig),
)

func newConfig() *viper.Viper {
	return viper.GetViper()
}
