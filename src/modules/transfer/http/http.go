package http

import (
	"context"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/src/modules/transfer/config"
	"github.com/didi/nightingale/src/modules/transfer/http/routes"
	"github.com/didi/nightingale/src/toolkits/address"
	"github.com/didi/nightingale/src/toolkits/http/middleware"
)

var srv = &http.Server{
	ReadTimeout:    10 * time.Second,
	WriteTimeout:   10 * time.Second,
	MaxHeaderBytes: 1 << 20,
}

// Start http server
func Start() {
	c := config.Config

	loggerMid := middleware.LoggerWithConfig(middleware.LoggerConfig{})
	recoveryMid := middleware.Recovery()

	if c.Logger.Level != "DEBUG" {
		gin.SetMode(gin.ReleaseMode)
		middleware.DisableConsoleColor()
	} else {
		srv.WriteTimeout = 120 * time.Second
	}

	r := gin.New()
	r.Use(loggerMid, recoveryMid)

	routes.Config(r)

	srv.Addr = address.GetHTTPListen("transfer")
	srv.Handler = r

	go func() {
		log.Println("starting http server, listening on:", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listening %s occur error: %s\n", srv.Addr, err)
		}
	}()
}

// Shutdown http server
func Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalln("cannot shutdown http server:", err)
	}

	// catching ctx.Done(). timeout of 5 seconds.
	select {
	case <-ctx.Done():
		log.Println("shutdown http server timeout of 5 seconds.")
	default:
		log.Println("http server stopped")
	}
}
