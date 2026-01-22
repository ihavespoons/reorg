package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	grpcserver "github.com/beng/reorg/internal/api/grpc"
	"github.com/beng/reorg/internal/api/rest"
	"github.com/beng/reorg/internal/service"
	"github.com/beng/reorg/internal/storage/markdown"
)

var (
	grpcPort string
	httpPort string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the reorg server",
	Long: `Start the reorg server in server mode.

This runs a gRPC server (default port 50051) and optionally a REST gateway
(default port 8080) that other clients can connect to.

Examples:
  reorg serve
  reorg serve --grpc-port 50051 --http-port 8080`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVar(&grpcPort, "grpc-port", "50051", "gRPC server port")
	serveCmd.Flags().StringVar(&httpPort, "http-port", "8080", "HTTP REST gateway port")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Check if initialized
	if _, err := os.Stat(filepath.Join(dataDir, "areas")); os.IsNotExist(err) {
		return fmt.Errorf("reorg not initialized. Run 'reorg init' first")
	}

	// Initialize store and local client
	store := markdown.NewStore(dataDir)
	localClient := service.NewLocalClient(store)

	// Create gRPC server
	grpcServer := grpcserver.NewServer(localClient)

	grpcAddress := ":" + grpcPort
	httpAddress := ":" + httpPort

	fmt.Println(titleStyle.Render("\n  Reorg Server\n"))
	fmt.Printf("Starting gRPC server on %s\n", grpcAddress)
	fmt.Printf("Starting REST gateway on %s\n", httpAddress)
	fmt.Printf("Data directory: %s\n\n", dataDir)

	// Handle shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 2)

	// Start gRPC server
	go func() {
		if err := grpcServer.Start(grpcAddress); err != nil {
			errCh <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Start REST gateway
	go func() {
		gateway := rest.NewGateway("localhost"+grpcAddress, httpAddress)
		if err := gateway.Start(ctx); err != nil {
			errCh <- fmt.Errorf("REST gateway error: %w", err)
		}
	}()

	// Wait for signal or error
	select {
	case sig := <-sigCh:
		fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
		return nil
	case err := <-errCh:
		return err
	}
}
