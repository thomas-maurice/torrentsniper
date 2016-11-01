package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/Shopify/sarama"
	"github.com/Sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/thomas-maurice/torrentsniper/common"
)

func InitConfig(configName string) error {
	viper.SetConfigName(configName)
	viper.AutomaticEnv()
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		return err
	}
	logrus.WithField("config_file", viper.ConfigFileUsed()).Info("Reading config")
	return nil
}

func main() {
	if err := InitConfig("torrentfetcher"); err != nil {
		logrus.Warningf("Could not load configuration file: %s", err)
	}
	doc, err := goquery.NewDocument("https://thepiratebay.org/top/48hall")

	if err != nil {
		logrus.Error("Could not get recent torrents: %s", err)
	}

	config := sarama.NewConfig()
	config.ClientID = viper.GetString("ClientID")
	config.Producer.Partitioner = sarama.NewRandomPartitioner
	config.Producer.Retry.Max = 10
	config.Producer.Retry.Backoff = time.Second * 6

	producer, err := sarama.NewSyncProducer([]string{viper.GetString("KafkaHost")}, config)
	if err != nil {
		logrus.Fatalf("Failed to start Sarama producer: %s", err)
	}

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		if link, has := s.Attr("href"); has {
			if strings.HasPrefix(link, "magnet:") {
				m := common.Torrent{
					Link:     link,
					Type:     "magnet",
					Source:   "The Pirate Bay",
					Category: "Unknown",
				}
				b, err := json.Marshal(m)
				if err != nil {
					logrus.Errorf("Could not marshal: %s", err)
				}
				message := &sarama.ProducerMessage{
					Topic: viper.GetString("TorrentLinksTopic"),
					Value: sarama.StringEncoder(bytes.NewBuffer(b).String()),
				}

				partition, offset, err := producer.SendMessage(message)
				logrus.Info(partition, offset, err, link)
			}
		}
	})

	producer.Close()

}
