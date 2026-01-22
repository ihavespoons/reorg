package rest

import (
	"context"
	"fmt"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/beng/reorg/api/proto/gen"
)

// Gateway provides a REST API via gRPC-Gateway
type Gateway struct {
	grpcAddress string
	httpAddress string
}

// NewGateway creates a new REST gateway
func NewGateway(grpcAddress, httpAddress string) *Gateway {
	return &Gateway{
		grpcAddress: grpcAddress,
		httpAddress: httpAddress,
	}
}

// Start starts the REST gateway server
func (g *Gateway) Start(ctx context.Context) error {
	mux := runtime.NewServeMux()

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := pb.RegisterReorgServiceHandlerFromEndpoint(ctx, mux, g.grpcAddress, opts); err != nil {
		return fmt.Errorf("failed to register gateway: %w", err)
	}

	server := &http.Server{
		Addr:    g.httpAddress,
		Handler: mux,
	}

	return server.ListenAndServe()
}
