package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Shopify/sarama"
	"github.com/Sirupsen/logrus"
	cluster "github.com/bsm/sarama-cluster"
	"github.com/olivere/elastic"
	"github.com/oschwald/geoip2-golang"
	"github.com/spf13/viper"
	"gopkg.in/redis.v5"

	"github.com/thomas-maurice/torrentsniper/common"
)

type location struct {
	Lon float64 `json:"lon"`
	Lat float64 `json:"lat"`
}

type enrichedMessage struct {
	common.Peer
	Location       location `json:"location"`
	CountryCode    string   `json:"country_code"`
	Country        string   `json:"country"`
	City           string   `json:"city"`
	ASNumber       uint     `json:"as_number"`
	ASOrganization string   `json:"as_organization"`
	ISP            string   `json:"isp"`
	Organization   string   `json:"organization"`
	ProcessedBy    string   `json:"processed_by"`
	ProcessedAt    int64    `json:"processed_at"`
	NameRaw        string   `json:"torrent_name_raw"` // So that it is not analyzed in ES
}

func InitConfig(configName string) error {
	viper.SetConfigName(configName)
	viper.AutomaticEnv()
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc")
	viper.SetDefault("ESAddress", "http://localhost:9200")
	viper.SetDefault("MaxMindDB", "GeoLite2-City.mmdb")
	viper.SetDefault("RedisAddress", "localhost:6379")
	viper.SetDefault("RedisPassword", "")
	viper.SetDefault("RedisDB", 0)
	viper.SetDefault("RedisConnectTimeout", "1s")
	viper.SetDefault("RedisCacheTTL", "3600s")
	viper.SetDefault("PromAddr", ":8080")

	err := viper.ReadInConfig()
	if err != nil {
		return err
	}
	return nil
}

func main() {
	if err := InitConfig("datapusher"); err != nil {
		logrus.Warningf("Could not load configuration file: %s", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		logrus.Fatalf("Could not get my hostname: %s", err)
	}

	common.StartPrometheusHandler(viper.GetString("PromAddr"))

	if viper.GetBool("UseRedis") {
		logrus.Infof("Using redis cache, at %s", viper.GetString("RedisAddress"))
	} else {
		logrus.Warningf("Not using redis cache !")
	}

	es, err := elastic.NewClient(elastic.SetURL(viper.GetString("ESAddress")))
	if err != nil {
		logrus.Fatalf("Could not create new ES client: %s", err)
	} else {
		logrus.Infof("Created new ES client, %s", viper.GetString("ESAddress"))
	}

	if exists, err := es.IndexExists(viper.GetString("ESIndex")).Do(); !exists {
		_, err = es.CreateIndex(viper.GetString("ESIndex")).Do()
		if err != nil {
			// Handle error
			panic(err)
		}
	}

	config := cluster.NewConfig()
	config.ClientID = viper.GetString("ClientID")
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	client, err := cluster.NewClient([]string{viper.GetString("KafkaHost")}, config)

	if err != nil {
		logrus.Fatalf("Could not initialize client: %s", err)
	}

	consumer, err := cluster.NewConsumerFromClient(client, "torrent-consumer", []string{viper.GetString("TorrentTopic")})

	if err != nil {
		logrus.Fatalf("Could not initiate consumer: %s", err)
	}

	defer consumer.Close()

	c := make(chan os.Signal, 1)

	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	signal.Notify(c, os.Interrupt, syscall.SIGKILL)

	db, err := geoip2.Open(viper.GetString("MaxmindDB"))

	if err != nil {
		logrus.Fatal("Could not open maxmind db")
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:        viper.GetString("RedisAddress"),
		Password:    viper.GetString("RedisPassword"),
		DB:          viper.GetInt("RedisDB"),
		DialTimeout: viper.GetDuration("RedisConnectTimeout"),
	})

	bulkProcessorService := es.BulkProcessor()
	bulkProcessorService.FlushInterval(time.Second * 10)
	bulkProcessorService.Before(func(executionId int64, requests []elastic.BulkableRequest) {
		logrus.Infof("Bulking request %d to ES", executionId)
	})
	bulkProcessorService.After(func(executionId int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
		if err != nil {
			logrus.Errorf("Error when bulking request to ES: %s, executionId: %d", err, executionId)
			elasticPushed.WithLabelValues("KO").Add(float64(len(requests)))
		} else {
			logrus.Infof("Bulked request to ES: %d - %d documents sent", executionId, len(requests))
			elasticPushed.WithLabelValues("OK").Add(float64(len(requests)))
		}
	})

	bulkProcessor, err := bulkProcessorService.Do()
	if err != nil {
		logrus.Fatalf("Could not create bulkProcessor: %s", err)
	}

	go func() {
		for _ = range c {
			logrus.Info("Gracefully shutting down")
			bulkProcessor.Flush()
			consumer.Close()
			os.Exit(0)
		}
	}()

	for {
		select {
		case msg := <-consumer.Messages():
			var m common.Peer
			err := json.Unmarshal(bytes.NewBufferString(string(msg.Value)).Bytes(), &m)
			if err != nil {
				logrus.Errorf("Could not unmarshal %s: %s", msg, err)

				consumer.MarkPartitionOffset(viper.GetString("TorrentTopic"), msg.Partition, msg.Offset, "")
				msgConsume.WithLabelValues("KO").Inc()
				continue
			}

			if viper.GetBool("UseRedis") {
				beforeCache := time.Now()

				_, err := redisClient.Get(fmt.Sprintf("%s-%s", m.IP, m.Torrent.Hash)).Result()
				if err == redis.Nil {
					ipsCache.WithLabelValues("MISSED").Inc()
				} else if err != nil {
					logrus.Warningf("Redis unreachable, processing. Got: %s", err)
				} else { // err == nil, we have the IP
					ipsCache.WithLabelValues("HIT").Inc()
					continue
				}

				logrus.Infof("Contacting redis took %s", time.Now().Sub(beforeCache))
			}

			ip := net.ParseIP(m.IP)
			record, err := db.City(ip)
			em := enrichedMessage{Peer: m}
			em.NameRaw = m.Name
			em.ProcessedAt = time.Now().Unix()
			em.ProcessedBy = hostname
			if err == nil {
				em.Location = location{Lat: record.Location.Latitude, Lon: record.Location.Longitude}
				em.City = record.City.Names["en"]
				em.CountryCode = record.Country.IsoCode
				em.Country = record.Country.Names["en"]
			} else {
				logrus.Warningf("Could not grab city info for %s: %s", em.IP, err)
			}

			/*ispInfo, err := db.ISP(ip)
			  if err == nil {
			      em.ISP = ispInfo.ISP
			      em.Organization = ispInfo.Organization
			      em.ASNumber = ispInfo.AutonomousSystemNumber
			      em.ASOrganization = ispInfo.AutonomousSystemOrganization
			  } else {
			      logrus.Warningf("Could not grab ISP info for %s: %s", em.IP, err)
			  }*/

			/*resp, err := es.Index().
			Index(viper.GetString("ESIndex")).
			Id(em.IP + "-" + em.TorrentHash).
			Type("address").
			BodyJson(em).
			Refresh(true).
			Do()*/
			documentId := fmt.Sprintf("%s-%s", em.IP, em.Torrent.Hash)
			bulkProcessor.Add(elastic.NewBulkIndexRequest().
				Index(viper.GetString("ESIndex")).
				Id(documentId).
				Type("address").
				Doc(em))

			/*if err != nil {
				logrus.Error("Could not update index", documentId, err)
			}*/

			logrus.Infof("Inserted document %s for bulk request - Partition %d offset %d", documentId, msg.Partition, msg.Offset)
			if viper.GetBool("UseRedis") {
				logrus.Infof("Caching key %s", documentId)
				err := redisClient.Set(documentId, fmt.Sprintf("%v", em), viper.GetDuration("RedisCacheTTL")).Err()
				if err != nil {
					logrus.Errorf("Could not update key %s: %s", documentId, err)
				} else {
					ipsCache.WithLabelValues("ADDED").Inc()
				}
			}

			msgConsume.WithLabelValues("OK").Inc()
			consumer.MarkPartitionOffset(viper.GetString("TorrentTopic"), msg.Partition, msg.Offset, "")

		case err := <-consumer.Errors():
			logrus.Errorf("Consumer error: %s", err)
		}
	}
}
