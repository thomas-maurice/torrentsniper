package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	msgConsume = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "datapusher",
		Name:      "msg_consumed",
		Help:      "Messages consumed",
	},
		[]string{"status"}) // OK | KO

	elasticPushed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "datapusher",
		Name:      "es_pushed",
		Help:      "Messages pushed to ElasticSearch",
	},
		[]string{"status"}) // OK | KO

	ipsCache = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "datapusher",
		Name:      "ips_cache",
		Help:      "Cached IP: MISSED, HIT or ADDED",
	},
		[]string{"status"}) // ADDED, HIT, MISSED
)

func init() {
	prometheus.MustRegister(msgConsume)
	prometheus.MustRegister(elasticPushed)
	prometheus.MustRegister(ipsCache)
}
