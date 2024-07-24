package collector

import (
	"context"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

var (
	// PingNowDesc 创建指标描述
	PingNowDesc = prometheus.NewDesc(namespace, "ping test", []string{"type"}, nil)
)

type ScraperPing struct{}

func (ScraperPing) Name() string {
	return "ping"
}

func (ScraperPing) Help() string {
	return "Collect from ping"
}

func (ScraperPing) Version() float64 {
	return 1.0
}

func (ScraperPing) Scrape(ctx context.Context, ch chan<- prometheus.Metric, logger log.Logger) error {
	// 执行数据采集并传递给prometheus处理
	// 做一个简单的延迟
	time.Sleep(time.Second)
	ch <- prometheus.MustNewConstMetric(PingNowDesc, prometheus.GaugeValue, 1, "ping")
	return nil
}

// 接口检查
var _ Scraper = ScraperPing{}
