package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"git.lunarlabs.dev/flavor/flavor/internal/app"
	"git.lunarlabs.dev/flavor/flavor/internal/logging"
	"git.lunarlabs.dev/flavor/flavor/internal/secret"
	"git.lunarlabs.dev/flavor/flavor/internal/session"
	"git.lunarlabs.dev/flavor/flavor/internal/version"
)

func main() {
	if err := session.PrepareProcessEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "flavord: prepare environment: %v\n", err)
		os.Exit(1)
	}
	runtimeDir := flag.String("runtime-dir", "", "runtime directory parent (defaults to $XDG_RUNTIME_DIR)")
	secretStore := flag.String("secret-store", "auto", "secret store backend: auto or memory")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn or error")
	experimentalDNS := flag.String("experimental-dns", "", "development only: serve synthetic DNS (IPv6) on this loopback address, e.g. 127.0.0.1:5353")
	syntheticHelper := flag.String("synthetic-helper", "", "experimental: use flavor-netd at this socket (e.g. /run/flavor/netd.sock) for system-wide Flavor names and addresses")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	info := version.Info()
	if *showVersion {
		fmt.Println(info.Line("flavord"))
		return
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "flavord: invalid --log-level %q\n", *logLevel)
		os.Exit(2)
	}
	if *syntheticHelper != "" && runtime.GOOS != "linux" {
		fmt.Fprintln(os.Stderr, "flavord: --synthetic-helper is only supported on Linux")
		os.Exit(2)
	}
	mode := secret.Mode(*secretStore)
	if mode != secret.ModeAuto && mode != secret.ModeMemory {
		fmt.Fprintf(os.Stderr, "flavord: invalid --secret-store %q\n", *secretStore)
		os.Exit(2)
	}
	log := logging.New(os.Stderr, level)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := app.Run(ctx, app.Options{RuntimeOverride: *runtimeDir, SecretMode: mode, Log: log, ExperimentalDNS: *experimentalDNS, SyntheticHelper: *syntheticHelper})
	if errors.Is(err, app.ErrAlreadyRunning) {
		log.Error(err.Error())
		os.Exit(3)
	}
	if err != nil {
		log.Error("flavord failed", "err", err.Error())
		os.Exit(1)
	}
}
