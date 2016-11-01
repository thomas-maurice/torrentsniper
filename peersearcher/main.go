package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Shopify/sarama"
	"github.com/Sirupsen/logrus"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/bsm/sarama-cluster"
	"github.com/spf13/viper"
	"gopkg.in/redis.v5"

	"github.com/thomas-maurice/torrentsniper/common"
)

func setDefaults() {
	viper.SetDefault("RedisAddress", "localhost:6379")
	viper.SetDefault("RedisPassword", "")
	viper.SetDefault("RedisDB", 0)
	viper.SetDefault("RedisConnectTimeout", "1s")
	viper.SetDefault("RedisCacheTTL", "3600s")
}

func main() {
	if err := common.InitConfig("peersearcher"); err != nil {
		logrus.WithError(err).Warning("Could not load configuration file")
	}

	setDefaults()

	common.StartPrometheusHandler(viper.GetString("PromAddr"))
	log.SetFlags(0)
	log.SetOutput(ioutil.Discard)

	config := sarama.NewConfig()
	config.ClientID = viper.GetString("ClientID")
	config.Producer.Partitioner = sarama.NewRandomPartitioner
	config.Producer.Retry.Max = 10
	config.Producer.Retry.Backoff = time.Second * 6

	logrus.Info("Starting producer")

	producer, err := sarama.NewAsyncProducer([]string{viper.GetString("KafkaHost")}, config)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to start Sarama producer")
	}

	defer func() {
		producer.AsyncClose()
	}()

	configConsumer := cluster.NewConfig()
	configConsumer.ClientID = viper.GetString("ClientID")
	configConsumer.Consumer.Offsets.Initial = sarama.OffsetOldest

	client, err := cluster.NewClient([]string{viper.GetString("KafkaHost")}, configConsumer)

	if err != nil {
		logrus.WithError(err).Fatalf("Could not initialize consumer")
	}

	consumer, err := cluster.NewConsumerFromClient(client, "torrent-link-consumer", []string{viper.GetString("TorrentLinksTopic")})

	if err != nil {
		logrus.WithError(err).Fatalf("Could not initiate consumer")
	}

	defer consumer.Close()

	c := make(chan os.Signal, 1)

	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	signal.Notify(c, os.Interrupt, syscall.SIGKILL)

	go func() {
		for _ = range c {
			logrus.Info("Gracefully shutting down")
			consumer.Close()
			os.Exit(0)
		}
	}()

	torrentClient, err := torrent.NewClient(&torrent.Config{
		DataDir:     "/tmp",
		NoUpload:    true,
		DisableIPv6: true,
		Debug:       false,
		ListenAddr:  "0.0.0.0:9999",
	})

	redisClient := redis.NewClient(&redis.Options{
		Addr:        viper.GetString("RedisAddress"),
		Password:    viper.GetString("RedisPassword"),
		DB:          viper.GetInt("RedisDB"),
		DialTimeout: viper.GetDuration("RedisConnectTimeout"),
	})

	defer redisClient.Close()

	defer torrentClient.Close()
	if err != nil {
		logrus.WithError(err).Fatal("Could not create client")
	}

	var stepStart time.Time
	for {
		workerActive.Set(0)
		processedIps := 0
		select {
		case msg := <-consumer.Messages():
			workerActive.Set(1)
			var torrentDesc common.Torrent
			stepStart = time.Now()
			err := json.Unmarshal(bytes.NewBufferString(string(msg.Value)).Bytes(), &torrentDesc)
			processingTime.WithLabelValues("unmarshal").Observe(float64(time.Since(stepStart)))
			if err != nil {
				logrus.WithError(err).Error("Could not unmarshal message")
				consumer.MarkPartitionOffset(viper.GetString("TorrentLinksTopic"), msg.Partition, msg.Offset, "")
				msgConsume.WithLabelValues("KO").Inc()
				msgConsume.WithLabelValues("KO_CouldNotUnmarshalMessage").Inc()
				continue
			}

			var tor *torrent.Torrent

			stepStart = time.Now()

			if torrentDesc.Type == "magnet" || torrentDesc.Type == "" { // Default assume mgnet
				logrus.WithField("link", torrentDesc.Link).Debug("Processing link: ")
				tor, err = torrentClient.AddMagnet(torrentDesc.Link)
				if err != nil {
					logrus.WithError(err).Fatalf("Could not add torrent")
					consumer.MarkPartitionOffset(viper.GetString("TorrentLinksTopic"), msg.Partition, msg.Offset, "")
					msgConsume.WithLabelValues("KO").Inc()
					msgConsume.WithLabelValues("KO_CouldNotAddTorrent").Inc()
					continue
				} else {
					logrus.WithField("torrent", tor.Name()).Info("Added torrent to client")
				}
				processingTime.WithLabelValues("handleMagnet").Observe(float64(time.Since(stepStart)))
			} else if torrentDesc.Type == "redis_base64" {
				cmd := redisClient.Get(torrentDesc.Data)
				if cmd.Err() == nil || cmd.Err() != redis.Nil {
					res, _ := cmd.Result()
					raw, err := base64.StdEncoding.DecodeString(res)
					if err != nil {
						logrus.WithError(err).Error("Could not unmarshal base64 from redis")
						consumer.MarkPartitionOffset(viper.GetString("TorrentLinksTopic"), msg.Partition, msg.Offset, "")
						msgConsume.WithLabelValues("KO").Inc()
						msgConsume.WithLabelValues("KO_InvalidBase64FromRedis").Inc()
						processingTime.WithLabelValues("handleTorrent").Observe(float64(time.Since(stepStart)))
						continue
					}
					mi, err := metainfo.Load(bytes.NewReader(raw))
					if err != nil {
						logrus.WithError(err).Error("Could not load metainfo from torrent_data")
						consumer.MarkPartitionOffset(viper.GetString("TorrentLinksTopic"), msg.Partition, msg.Offset, "")
						msgConsume.WithLabelValues("KO").Inc()
						msgConsume.WithLabelValues("KO_CouldNotLoadMetadata").Inc()
						processingTime.WithLabelValues("handleTorrent").Observe(float64(time.Since(stepStart)))
						continue
					}
					tor, err = torrentClient.AddTorrent(mi)
					if err != nil {
						logrus.WithError(err).Error("Could not add torrent from data")
						consumer.MarkPartitionOffset(viper.GetString("TorrentLinksTopic"), msg.Partition, msg.Offset, "")
						msgConsume.WithLabelValues("KO").Inc()
						msgConsume.WithLabelValues("KO_CouldNotAddTorrent").Inc()
						processingTime.WithLabelValues("handleTorrent").Observe(float64(time.Since(stepStart)))
						continue
					}
				} else {
					logrus.WithError(err).Error("Could not add torrent from data")
					consumer.MarkPartitionOffset(viper.GetString("TorrentLinksTopic"), msg.Partition, msg.Offset, "")
					msgConsume.WithLabelValues("KO").Inc()
					msgConsume.WithLabelValues("KO_CouldNotAddTorrent").Inc()
					processingTime.WithLabelValues("handleTorrent").Observe(float64(time.Since(stepStart)))
					continue
				}
			}

			stepStart = time.Now()
			hash := tor.InfoHash()

			dht := torrentClient.DHT()

			announce, err := dht.Announce(hash.AsString(), 9999, true)

			if err != nil {
				msgConsume.WithLabelValues("KO").Inc()
				msgConsume.WithLabelValues("KO_CouldNotGetAnnounce").Inc()
				logrus.WithField("torrent", tor.Name()).Errorf("Could not get announce: %s", err)
				// Mark offset & exit the processing
				consumer.MarkPartitionOffset(viper.GetString("TorrentLinksTopic"), msg.Partition, msg.Offset, "")
				continue
			} else {
				logrus.WithField("torrent", tor.Name()).Info("Got announce from trackers")
			}

			processingTime.WithLabelValues("dhtAnnounce").Observe(float64(time.Since(stepStart)))
			for {
				p, more := <-announce.Peers
				if more == false {
					logrus.WithField("torrent", tor.Name()).Info("No more peers to process, channel empty")
					break
				}
				for _, peer := range p.Peers {
					stepStart = time.Now()
					torrentDesc.Name = tor.Name()
					torrentDesc.Hash = tor.InfoHash().HexString()
					ms := common.Peer{
						IP:      peer.IP.String(),
						Torrent: torrentDesc,
						SeenAt:  time.Now().Unix(),
					}
					b, err := json.Marshal(ms)
					if err != nil {
						logrus.WithError(err).Error("Could not marshal ip info")
						msgConsume.WithLabelValues("KO").Inc()
						msgConsume.WithLabelValues("KO_CouldNotMarshalIP").Inc()
						continue
					}

					message := &sarama.ProducerMessage{
						Topic: viper.GetString("TorrentTopic"),
						Value: sarama.StringEncoder(bytes.NewBuffer(b).String()),
					}

					producer.Input() <- message
					processedIps += 1
					logrus.WithFields(logrus.Fields{"peerIP": peer.IP, "torrent": tor.Name()}).Debug("Sent peer")
					ipGathered.Inc()
					processingTime.WithLabelValues("processPeer").Observe(float64(time.Since(stepStart)))
				}

			}
			consumer.MarkPartitionOffset(viper.GetString("TorrentLinksTopic"), msg.Partition, msg.Offset, "")
			msgConsume.WithLabelValues("OK").Inc()
			logrus.WithField("torrent", tor.Name()).Infof("Sent %d IPs", processedIps)
			logrus.WithField("torrent", tor.Name()).Info("Done processing")
			cleanTorrentClient(torrentClient)

		case err := <-consumer.Errors():
			logrus.WithError(err).Error("Consumer error")

		}
	}
}

func cleanTorrentClient(client *torrent.Client) {
	stepStart := time.Now()
	for _, torrent := range client.Torrents() {
		torrent.Drop()
	}
	processingTime.WithLabelValues("torrentClientClean").Observe(float64(time.Since(stepStart)))
}
