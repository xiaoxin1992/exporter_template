package collector

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
	"time"
)

const exporter = "exporter"

var (
	upExporter             = prometheus.NewDesc(prometheus.BuildFQName(namespace, "", "up"), "Whether the exporter is up", nil, nil)
	ScrapeCollectorSuccess = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, exporter, "collector_success"),
		"mysqld_exporter: Whether a collector succeeded.",
		[]string{"collector"},
		nil,
	)
	ScrapeDurationSeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, exporter, "collector_duration_seconds"),
		"Collector time duration.",
		[]string{"collector"}, nil,
	)
)

type Exporter struct {
	ctx      context.Context
	logger   log.Logger
	scrapers []Scraper
}

func New(ctx context.Context, scraper []Scraper, logger log.Logger) *Exporter {
	return &Exporter{
		ctx:      ctx,
		logger:   logger,
		scrapers: scraper,
	}
}

func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	// 描述信息写入并且传递给prometheus的chan处理
	ch <- upExporter

}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	// 执行采集模块
	up := e.scrape(e.ctx, ch)
	ch <- prometheus.MustNewConstMetric(upExporter, prometheus.GaugeValue, up)
}

func (e *Exporter) scrape(ctx context.Context, ch chan<- prometheus.Metric) float64 {
	var wg sync.WaitGroup
	defer wg.Wait()
	for _, scraper := range e.scrapers {
		// 所有采集模块在此执行，执行过程中等待结束
		wg.Add(1)
		go func(scraper Scraper) {
			defer wg.Done()
			label := fmt.Sprintf("collect.%s", scraper.Name())
			scrapeTime := time.Now()
			collectorSuccess := 1.0
			if err := scraper.Scrape(ctx, ch, log.With(e.logger, "scraper", scraper.Name())); err != nil {
				level.Error(e.logger).Log("msg", "Error from scraper", "scraper", scraper.Name(), "target", "err", err)
				collectorSuccess = 0.0
			}
			ch <- prometheus.MustNewConstMetric(ScrapeCollectorSuccess, prometheus.GaugeValue, collectorSuccess, label)
			ch <- prometheus.MustNewConstMetric(ScrapeDurationSeconds, prometheus.GaugeValue, time.Since(scrapeTime).Seconds(), label)
		}(scraper)
	}
	return 1.0
}
