package common

import (
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
)

func InitConfig(configName string) error {
	viper.SetConfigName(configName)
	viper.AutomaticEnv()
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc")
	viper.AddConfigPath(".")

	setDefaults()
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}
	return nil
}

func setDefaults() {
	viper.SetDefault("PromAddr", ":8080")
}

func StartPrometheusHandler(addr string) {

	go func() {
		logrus.WithField("PromAddr", addr).Info("start prometheus handler /metrics")

		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(addr, nil)
	}()
}
