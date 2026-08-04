package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gamp/forgetss/internal/api"
	"github.com/gamp/forgetss/internal/channelaccounts"
	"github.com/gamp/forgetss/internal/config"
	"github.com/gamp/forgetss/internal/metrics"
	"github.com/gamp/forgetss/internal/rpc"
	"github.com/gamp/forgetss/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})))

	db, err := store.NewPostgres(cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	storeInst := store.New(db)

	metrics.Register()

	horizonEndpoints := cfg.HorizonEndpoints
	if len(horizonEndpoints) == 0 {
		horizonEndpoints = []string{"https://horizon-testnet.stellar.org"}
	}
	sorobanEndpoints := cfg.SorobanEndpoints
	if len(sorobanEndpoints) == 0 {
		sorobanEndpoints = []string{"https://soroban-testnet.stellar.org"}
	}

	router := rpc.NewRouter(horizonEndpoints, sorobanEndpoints)

	channelPool := channelaccounts.NewPool(storeInst, router, cfg.MasterSeed, cfg.RefillBatchSize)
	submissionEngine := submission.NewEngine(storeInst, router, channelPool, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		slog.Info("starting submission queue processor")
		if err := submissionEngine.ProcessQueue(ctx); err != nil {
			slog.Error("submission queue processor exited", "error", err)
		}
	}()

	srv := api.NewServer(cfg, storeInst, router, channelPool, submissionEngine)
	if err := srv.Start(ctx); err != nil {
		slog.Error("API server exited", "error", err)
		os.Exit(1)
	}
}
