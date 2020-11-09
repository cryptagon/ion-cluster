package cluster

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	prometheusGaugeSessions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ion_cluster_sessions",
			Help: "Number of currently active sessions on this node",
		},
	)

	prometheusGaugeProxyClients = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ion_cluster_proxy_clients",
			Help: "Number of currently active proxied websockets on this node",
		},
	)
)

func init() {
	prometheus.MustRegister(prometheusGaugeSessions)
	prometheus.MustRegister(prometheusGaugeProxyClients)
	prometheus.MustRegister(prometheus.NewBuildInfoCollector())
}

func metricsHandler() http.Handler {
	return promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	)
}
