package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"git.lunarlabs.dev/lattice/lattice/internal/app"
	"git.lunarlabs.dev/lattice/lattice/internal/logging"
	"git.lunarlabs.dev/lattice/lattice/internal/secret"
	"git.lunarlabs.dev/lattice/lattice/internal/session"
	"git.lunarlabs.dev/lattice/lattice/internal/version"
)

func main() {
	if err := session.PrepareProcessEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "latticed: prepare environment: %v\n", err)
		os.Exit(1)
	}
	runtimeDir := flag.String("runtime-dir", "", "runtime directory parent (defaults to $XDG_RUNTIME_DIR)")
	secretStore := flag.String("secret-store", "auto", "secret store backend: auto or memory")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn or error")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	info := version.Info()
	if *showVersion {
		fmt.Printf("latticed %s protocol %d.%d\n", info.DaemonVersion, info.ProtocolMajor, info.ProtocolMinor)
		return
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "latticed: invalid --log-level %q\n", *logLevel)
		os.Exit(2)
	}
	mode := secret.Mode(*secretStore)
	if mode != secret.ModeAuto && mode != secret.ModeMemory {
		fmt.Fprintf(os.Stderr, "latticed: invalid --secret-store %q\n", *secretStore)
		os.Exit(2)
	}
	log := logging.New(os.Stderr, level)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := app.Run(ctx, app.Options{RuntimeOverride: *runtimeDir, SecretMode: mode, Log: log})
	if errors.Is(err, app.ErrAlreadyRunning) {
		log.Error(err.Error())
		os.Exit(3)
	}
	if err != nil {
		log.Error("latticed failed", "err", err.Error())
		os.Exit(1)
	}
}
