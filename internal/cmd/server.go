package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/xiehqing/hiagent-core/internal/db"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/xiehqing/hiagent-core/internal/config"
	crushlog "github.com/xiehqing/hiagent-core/internal/log"
	"github.com/xiehqing/hiagent-core/internal/server"
)

var serverHost string

func init() {
	serverCmd.Flags().StringVarP(&serverHost, "host", "H", server.DefaultHost(), "Server host (TCP or Unix socket)")
	rootCmd.AddCommand(serverCmd)
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the HiAgent server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		dataDir, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			return fmt.Errorf("failed to get data directory: %v", err)
		}
		debug, err := cmd.Flags().GetBool("debug")
		if err != nil {
			return fmt.Errorf("failed to get debug flag: %v", err)
		}

		driver, _ := cmd.Flags().GetString("driver")
		dsn, _ := cmd.Flags().GetString("dsn")

		dataDir = config.DefaultDataDir(config.GlobalWorkspaceDir(), dataDir)

		conn, err := db.ConnectWithOption(cmd.Context(), driver, dataDir, dsn)
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		cfg, err := config.Load(config.GlobalWorkspaceDir(), dataDir, conn, debug)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %v", err)
		}

		logFile := filepath.Join(config.GlobalCacheDir(), "server-"+safeNameRegexp.ReplaceAllString(serverHost, "_"), "hi_agent.log")

		if term.IsTerminal(os.Stderr.Fd()) {
			crushlog.Setup(logFile, debug, os.Stderr)
		} else {
			crushlog.Setup(logFile, debug)
		}

		hostURL, err := server.ParseHostURL(serverHost)
		if err != nil {
			return fmt.Errorf("invalid server host: %v", err)
		}

		srv := server.NewServer(cfg, hostURL.Scheme, hostURL.Host)
		srv.SetLogger(slog.Default())
		slog.Info("Starting Crush server...", "addr", serverHost)

		errch := make(chan error, 1)
		sigch := make(chan os.Signal, 1)
		sigs := []os.Signal{os.Interrupt}
		sigs = append(sigs, addSignals(sigs)...)
		signal.Notify(sigch, sigs...)

		go func() {
			errch <- srv.ListenAndServe()
		}()

		select {
		case <-sigch:
			slog.Info("Received interrupt signal...")
		case err = <-errch:
			if err != nil && !errors.Is(err, server.ErrServerClosed) {
				_ = srv.Close()
				slog.Error("Server error", "error", err)
				return fmt.Errorf("server error: %v", err)
			}
		}

		if errors.Is(err, server.ErrServerClosed) {
			return nil
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()

		slog.Info("Shutting down...")

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Failed to shutdown server", "error", err)
			return fmt.Errorf("failed to shutdown server: %v", err)
		}

		return nil
	},
}
