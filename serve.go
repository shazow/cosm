package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/fvbock/endless"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

func serve(ctx context.Context, options Options) error {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	rtc := rtcServer{
		Logger: logger,
		HandleConnection: func(conn rtcConn) {
			logger.Info().Interface("conn", conn).Msg("new connection")
			if err := conn.DataChannel.SendText("hello"); err != nil {
				logger.Error().Err(err).Msg("data channel send failed")
			}
		},
	}
	rtc.init()

	r.Get("/rtc", rtc.ServeHTTP)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/index.html")
	})

	// TODO: endless doesn't support clean shutdown? With stdlib http, could do
	// <-ctx.Done(); server.Shutdown(context.Background())

	bind := options.Serve.Bind
	fmt.Fprintf(os.Stderr, "listening on %s\n", bind)
	return endless.ListenAndServe(bind, r)
}
