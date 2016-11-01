package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	msgConsume = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "peersearcher",
		Name:      "msg_consume",
		Help:      "Messages consumed",
	},
		[]string{"status"}) // OK | KO

	processingTime = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: "peersearcher",
			Name:      "processing_time_ns",
			Help:      "How much time it take to process a msg per step",
		},
		[]string{"step"},
	)
	workerActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "peersearcher",
		Name:      "active",
		Help:      "Current state of the worker (active/inactive)",
	})
	ipGathered = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "peersearcher",
		Name:      "ip_gathered",
		Help:      "IP gathered from trackers",
	})
)

func init() {
	prometheus.MustRegister(msgConsume)
	prometheus.MustRegister(processingTime)
	prometheus.MustRegister(workerActive)
}
