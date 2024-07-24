package main

import (
	"context"
	"exporter_template/collector"
	"fmt"
	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	name        = "exporter_template"
	description = "Prometheus Exporter for Template"
)

var (
	metricsPath   = kingpin.Flag("web.metrics.path", "Path under which to expose metrics.").Default("/metrics").String()
	toolKigFlag   = webflag.AddFlags(kingpin.CommandLine, ":9104")
	timeoutOffset = kingpin.Flag("timeout-offset", "Offset to subtract from timeout in seconds.").Default("0.25").Float64()
)

// 用于开启或关闭采集指标
var scrapers = map[collector.Scraper]bool{
	collector.ScraperPing{}: true,
}

func main() {
	scraperFlags := map[collector.Scraper]*bool{}
	for scraper, enabledByDefault := range scrapers {
		defaultOn := "false"
		if enabledByDefault {
			defaultOn = "true"
		}

		f := kingpin.Flag(
			"collect."+scraper.Name(),
			scraper.Help(),
		).Default(defaultOn).Bool()

		scraperFlags[scraper] = f
	}
	promLogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promLogConfig)
	kingpin.Version(version.Print(name))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promLogConfig)
	level.Info(logger).Log("msg", fmt.Sprintf("Starting %s", name), "version", version.Info())
	level.Info(logger).Log("msg", fmt.Sprintf("Build context: %s", version.BuildContext()))
	var enabledScrapers []collector.Scraper
	for scraper, enabled := range scraperFlags {
		if *enabled {
			level.Info(logger).Log("msg", "Scraper enabled", "scraper", scraper.Name())
			enabledScrapers = append(enabledScrapers, scraper)
		}
	}
	// 执行数据采集
	handlerFunc := newHandler(enabledScrapers, logger)
	http.Handle(*metricsPath, promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handlerFunc))
	// 帮助页面
	HelpPage(*metricsPath, &logger)
	//  启动http服务
	srv := &http.Server{}
	if err := web.ListenAndServe(srv, toolKigFlag, logger); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
		os.Exit(1)
	}
}

func newHandler(scrapers []collector.Scraper, logger log.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/*
			TODO 在这里执行采集数据，可以通过params传递采集需要的参数,通过q.Get获取
		*/
		q := r.URL.Query()
		collects := q["collect[]"]
		ctx := r.Context()

		// 设置timeout时间
		timeoutSeconds, err := getScrapeTimeoutSeconds(r, *timeoutOffset)
		if err != nil {
			level.Error(logger).Log("msg", "Error getting timeout from Prometheus header", "err", err)
		}
		if timeoutSeconds > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds*float64(time.Second)))
			defer cancel()
			r = r.WithContext(ctx)
		}
		// 过滤指定模块，如果没有传入指定模块则返回所有启用的模块
		filteredScrapers := filterScrapers(scrapers, collects)

		// 注册采集模块
		registry := prometheus.NewRegistry()
		registry.MustRegister(collector.New(ctx, filteredScrapers, logger))

		// 触发采集功能
		gatherers := prometheus.Gatherers{
			//prometheus.DefaultGatherer,
			registry,
		}
		// 将http服务委托给Prometheus客户端库，该库将调用收集器, 收集。
		h := promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{})
		h.ServeHTTP(w, r)
	}
}

func filterScrapers(scrapers []collector.Scraper, collectParams []string) []collector.Scraper {
	/*
	 过滤要执行的采集模块，可以通过http get的方式参数进来
	*/
	var filteredScrapers []collector.Scraper
	if len(collectParams) > 0 {
		filters := make(map[string]bool)
		for _, param := range collectParams {
			filters[param] = true
		}
		for _, scraper := range scrapers {
			if filters[scraper.Name()] {
				filteredScrapers = append(filteredScrapers, scraper)
			}
		}
	}
	if len(filteredScrapers) == 0 {
		return scrapers
	}
	return filteredScrapers
}

func getScrapeTimeoutSeconds(r *http.Request, offset float64) (float64, error) {
	/*
		计算timeout时间
	*/
	var timeoutSeconds float64
	if v := r.Header.Get("X-Prometheus-Scrape-Timeout-Seconds"); v != "" {
		var err error
		timeoutSeconds, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse timeout from Prometheus header: %v", err)
		}
	}
	if timeoutSeconds == 0 {
		return 0, nil
	}
	if timeoutSeconds < 0 {
		return 0, fmt.Errorf("timeout value from Prometheus header is invalid: %f", timeoutSeconds)
	}

	if offset >= timeoutSeconds {
		// Ignore timeout offset if it doesn't leave time to scrape.
		return 0, fmt.Errorf("timeout offset (%f) should be lower than prometheus scrape timeout (%f)", offset, timeoutSeconds)
	} else {
		// Subtract timeout offset from timeout.
		timeoutSeconds -= offset
	}
	return timeoutSeconds, nil
}

func HelpPage(metricsPath string, logger *log.Logger) {
	/*
		当metrics路径不是"/"并且也不为空的时候者生成一个"/"的帮助页面
	*/
	if metricsPath != "/" && metricsPath != "" {
		landingConfig := web.LandingConfig{
			Name:        name,
			Description: description,
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{Address: metricsPath, Text: "Metrics"},
			},
		}
		landingPage, err := web.NewLandingPage(landingConfig)
		if err != nil {
			level.Error(*logger).Log("msg", "Error creating landing page", "err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}
	return
}

func init() {
	prometheus.MustRegister(versioncollector.NewCollector(name))
}
