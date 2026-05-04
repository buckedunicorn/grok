// Package grpc provides a gRPC client for the xAI API.
//
// The gRPC surface mirrors the REST inference endpoints and is served at api.x.ai:443.
// Proto definitions are from https://github.com/xai-org/xai-proto; the generated Go
// code lives in the gen/ subdirectory and is committed to this repo.
//
// Usage:
//
//	gc, err := client.NewGRPC()
//	ctx := gc.AuthContext(context.Background())
//	resp, err := gc.Chat.GetCompletion(ctx, &xaiv1.GetCompletionsRequest{...})
package grpc

import (
	"context"
	"fmt"

	xaiv1 "github.com/buckedunicorn/grok/grpc/gen/xai/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

const defaultEndpoint = "api.x.ai:443"

// Client is the gRPC client for the xAI API.
// Each field is a generated service client ready to use after AuthContext(ctx).
type Client struct {
	// Chat provides text completion, streaming, and deferred completion RPCs.
	Chat xaiv1.ChatClient
	// Batch provides batch job management RPCs.
	Batch xaiv1.BatchMgmtClient
	// Models lists available language, embedding, and image generation models.
	Models xaiv1.ModelsClient
	// Images provides image generation RPCs.
	Images xaiv1.ImageClient
	// Files provides file upload, download, and management RPCs.
	Files xaiv1.FilesClient
	// Video provides async video generation and polling RPCs.
	Video xaiv1.VideoClient
	// Embedder provides text embedding RPCs.
	Embedder xaiv1.EmbedderClient
	// Documents provides RAG document search RPCs (gRPC-only, no REST equivalent).
	Documents xaiv1.DocumentsClient
	// Sample provides raw text sampling RPCs (gRPC-only, no REST equivalent).
	Sample xaiv1.SampleClient
	// Tokenize provides text tokenization RPCs (gRPC-only, no REST equivalent).
	Tokenize xaiv1.TokenizeClient
	// Auth provides API key introspection (gRPC-only).
	Auth xaiv1.AuthClient

	conn   *grpc.ClientConn
	apiKey string
}

// New creates a gRPC Client connected to api.x.ai:443.
// Additional dial options (e.g. interceptors) may be passed via opts.
func New(apiKey string, opts ...grpc.DialOption) (*Client, error) {
	base := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")),
	}
	conn, err := grpc.NewClient(defaultEndpoint, append(base, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("grpc.New: %w", err)
	}
	return &Client{
		Chat:      xaiv1.NewChatClient(conn),
		Batch:     xaiv1.NewBatchMgmtClient(conn),
		Models:    xaiv1.NewModelsClient(conn),
		Images:    xaiv1.NewImageClient(conn),
		Files:     xaiv1.NewFilesClient(conn),
		Video:     xaiv1.NewVideoClient(conn),
		Embedder:  xaiv1.NewEmbedderClient(conn),
		Documents: xaiv1.NewDocumentsClient(conn),
		Sample:    xaiv1.NewSampleClient(conn),
		Tokenize:  xaiv1.NewTokenizeClient(conn),
		Auth:      xaiv1.NewAuthClient(conn),
		conn:      conn,
		apiKey:    apiKey,
	}, nil
}

// AuthContext appends the Bearer token to an outgoing gRPC context.
// Pass the returned context to every RPC call:
//
//	ctx := gc.AuthContext(context.Background())
//	resp, err := gc.Chat.GetCompletion(ctx, req)
func (c *Client) AuthContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.apiKey)
}

// Conn returns the underlying *grpc.ClientConn for advanced use cases
// (e.g. constructing billing service clients from grpc/gen/xai/management_api).
func (c *Client) Conn() *grpc.ClientConn { return c.conn }

// Close shuts down the underlying gRPC connection.
func (c *Client) Close() error { return c.conn.Close() }
