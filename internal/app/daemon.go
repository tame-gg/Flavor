package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/config"
	"git.lunarlabs.dev/flavor/flavor/internal/events"
	"git.lunarlabs.dev/flavor/flavor/internal/ipc/server"
	"git.lunarlabs.dev/flavor/flavor/internal/ipc/transport"
	"git.lunarlabs.dev/flavor/flavor/internal/secret"
	"git.lunarlabs.dev/flavor/flavor/internal/service"
	"git.lunarlabs.dev/flavor/flavor/internal/session"
	"git.lunarlabs.dev/flavor/flavor/internal/store"
)

var ErrAlreadyRunning = errors.New("another flavord instance is running for this user")

type Options struct {
	Env             *config.Env
	RuntimeOverride string
	SecretMode      secret.Mode
	Log             *slog.Logger
	EngineFactory   session.EngineFactory
	ShutdownTimeout time.Duration
	OnReady         func(config.Paths)
	ExperimentalDNS string
	OnDNSReady      func(net.Addr)
	TUN             io.ReadWriteCloser
	SyntheticHelper string
}

func Run(ctx context.Context, opts Options) error {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 20 * time.Second
	}
	instanceID := newInstanceID()

	var paths config.Paths
	var err error
	if opts.Env != nil {
		paths, err = config.Resolve(*opts.Env)
	} else {
		paths, err = config.ResolveFromOS(opts.RuntimeOverride)
	}
	if err != nil {
		return fmt.Errorf("resolve paths: %w", err)
	}
	if err := migrateFromLattice(paths, log); err != nil {
		return fmt.Errorf("migrate Lattice data: %w", err)
	}
	if err := paths.EnsureFlavorDirs(); err != nil {
		return fmt.Errorf("prepare directories: %w", err)
	}

	unlock, err := lock(paths.Lock)
	if err != nil {
		return err
	}
	defer unlock()
	if err := removeStaleSocket(paths.Socket); err != nil {
		return err
	}

	db, err := store.Open(ctx, paths.Database)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := store.SweepDeletedIdentities(paths.NetworksRoot); err != nil {
		log.Error("sweeping deleted identities failed", "err", err.Error())
	}

	secrets, err := secret.Open(ctx, secret.Options{Mode: opts.SecretMode})
	if err != nil {
		return fmt.Errorf("open secret store: %w", err)
	}

	bus := events.NewBus(events.DefaultReplayCapacity, events.DefaultSubQueue)
	sessions := session.NewManager(bus, log)
	if opts.EngineFactory != nil {
		sessions = session.NewManagerWithFactory(bus, log, opts.EngineFactory)
	}
	svc := service.New(service.Config{
		Store:      db,
		Paths:      paths,
		Sessions:   sessions,
		Bus:        bus,
		Secrets:    secrets,
		Log:        log,
		InstanceID: instanceID,
	})

	l, err := transport.Listen(paths.Socket)
	if err != nil {
		return fmt.Errorf("bind ipc socket: %w", err)
	}
	httpSrv := server.New(svc)
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(l) }()
	log.Info("flavord ready", "socket", paths.Socket, "instance_id", instanceID)
	if opts.OnReady != nil {
		opts.OnReady(paths)
	}

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	stopSynthetic, err := startSynthetic(runCtx, opts, db, svc, sessions, log)
	if err != nil {
		cancelRun()
		_ = httpSrv.Close()
		return fmt.Errorf("synthetic addressing: %w", err)
	}
	go svc.ReconcileAutoConnect(runCtx)

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("ipc server: %w", err)
		}
	}
	cancelRun()
	stopSynthetic()

	log.Info("flavord shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
	defer cancel()
	if err := svc.Shutdown(shutdownCtx); err != nil {
		log.Error("stopping network sessions did not finish", "err", err.Error())
	}
	bus.Close()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		_ = httpSrv.Close()
	}
	return runErr
}

func newInstanceID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func lock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open instance lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("acquire instance lock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func removeStaleSocket(path string) error {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to replace non-socket at %s", path)
	}
	return os.Remove(path)
}
