package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/didi/nightingale/src/modules/judge/backend/query"
	redisp "github.com/didi/nightingale/src/modules/judge/backend/redis"
	"github.com/didi/nightingale/src/modules/judge/cache"
	"github.com/didi/nightingale/src/modules/judge/config"
	"github.com/didi/nightingale/src/modules/judge/cron"
	"github.com/didi/nightingale/src/modules/judge/http"
	"github.com/didi/nightingale/src/modules/judge/logger"
	"github.com/didi/nightingale/src/modules/judge/rpc"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/runner"
)

const version = 1

var (
	vers *bool
	help *bool
	conf *string
)

func init() {
	vers = flag.Bool("v", false, "display the version.")
	help = flag.Bool("h", false, "print this help.")
	conf = flag.String("f", "", "specify configuration file.")
	flag.Parse()

	if *vers {
		fmt.Println("version:", version)
		os.Exit(0)
	}

	if *help {
		flag.Usage()
		os.Exit(0)
	}
}

func main() {
	aconf()
	pconf()
	start()

	config.InitLogger()

	cfg := config.Config
	ident, err := config.GetIdentity(cfg.Identity)
	if err != nil {
		log.Fatalln("[F] cannot get identity:", err)
	}

	port, err := config.GetPort(cfg.Rpc.Listen)
	if err != nil {
		log.Fatalln("[F] cannot get identity:", err)
	}

	config.Identity = ident + ":" + port
	log.Printf("[I] identity -> %s", config.Identity)

	query.InitConnPools()
	cache.InitHistoryBigMap()
	cache.Strategy = cache.NewStrategyMap()
	cache.NodataStra = cache.NewStrategyMap()
	cache.SeriesMap = cache.NewIndexMap()

	// 初始化publisher组件
	switch cfg.Publisher.Type {
	case "redis":
		redisp.Pub, err = redisp.NewRedisPublisher(cfg.Publisher.Redis)
	default:
		err = errors.New("unknown publish type")
	}
	if err != nil {
		log.Fatalln("[F] init publisher failed:", err)
	}

	go http.Start(cfg.Http.Listen, cfg.Logger.Level)
	go rpc.Start()
	go cron.Report(ident, port, cfg.Report.Addrs, cfg.Report.Interval)
	go cron.Statstic()
	go cron.GetStrategy()
	go cron.NodataJudge()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	select {
	case <-c:
		logger.Info(0, "stop signal caught, try to stop judge server")
	}
	logger.Info(0, "judge server stopped succefully")
	logger.Close()
}

// auto detect configuration file
func aconf() {
	if *conf != "" && file.IsExist(*conf) {
		return
	}

	*conf = "etc/judge.local.yml"
	if file.IsExist(*conf) {
		return
	}

	*conf = "etc/judge.yml"
	if file.IsExist(*conf) {
		return
	}

	fmt.Println("no configuration file for judge")
	os.Exit(1)
}

// parse configuration file
func pconf() {
	if err := config.Parse(*conf); err != nil {
		fmt.Println("cannot parse configuration file:", err)
		os.Exit(1)
	}
}

func start() {
	runner.Init()
	fmt.Println("transfer start, use configuration file:", *conf)
	fmt.Println("runner.Cwd:", runner.Cwd)
	fmt.Println("runner.Hostname:", runner.Hostname)
}
