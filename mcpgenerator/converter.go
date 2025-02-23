package mcpgenerator

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/centralmind/gateway/connectors"
	"github.com/centralmind/gateway/model"
	"github.com/centralmind/gateway/plugins"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/xerrors"
)

type MCPServer struct {
	server *server.MCPServer
}

func New(
	schema model.Config,
) (*MCPServer, error) {
	srv := server.NewMCPServer("mcp-data-gateway", "0.0.1")
	var interceptors []plugins.Interceptor
	for k, v := range schema.Plugins {
		interceptor, err := plugins.New(k, v)
		if err != nil {
			return nil, err
		}
		interceptors = append(interceptors, interceptor)
	}
	connector, err := connectors.New(schema.Database.Type, schema.Database.Connection)
	if err != nil {
		return nil, xerrors.Errorf("unable to init connector: %w", err)
	}
	for _, info := range schema.Database.Tables {
		for _, endpoint := range info.Endpoints {
			var opts []mcp.ToolOption
			for _, col := range endpoint.Params {
				if col.Required {
					opts = append(opts, ArgumentOption(col, mcp.Required()))
				} else {
					opts = append(opts, ArgumentOption(col))
				}
			}

			srv.AddTool(mcp.NewTool(
				endpoint.MCPMethod,
				opts...,
			), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				res, err := connector.Query(ctx, endpoint, request.Params.Arguments)
				if err != nil {
					return &mcp.CallToolResult{
						Content: []interface{}{
							mcp.TextContent{
								Type: "text",
								Text: fmt.Sprintf("Unable to construct storage: %s", err),
							},
						},
						IsError: true,
					}, nil
				}
				var content []interface{}
				content = append(content, mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Found a %s row-(s) in %s.", len(res), info.Name),
				})
				for _, row := range res {
					content = append(content, mcp.TextContent{
						Type: "text",
						Text: jsonify(row),
					})
				}

				return &mcp.CallToolResult{
					Content: content,
				}, nil
			})
		}
	}

	return &MCPServer{
		server: srv,
	}, nil
}

func jsonify(data any) string {
	res, _ := json.Marshal(data)
	return string(res)
}

func (s *MCPServer) ServeSSE(addr string) *server.SSEServer {
	return server.NewSSEServer(s.server, fmt.Sprintf("http://%s", addr))
}

func (s *MCPServer) ServeStdio() *server.StdioServer {
	return server.NewStdioServer(s.server)
}

func ArgumentOption(col model.EndpointParams, opts ...mcp.PropertyOption) mcp.ToolOption {
	opts = append(opts, mcp.Title(fmt.Sprintf("Column %s", col.Name)))
	opts = append(opts, func(m map[string]interface{}) {
		m["default"] = col.Default
	})

	switch col.Type {
	case "integer", "double", "float", "number":
		return mcp.WithNumber(col.Name, opts...)

	case "string":
		return mcp.WithString(col.Name, opts...)

	case "bool", "boolean":
		return mcp.WithBoolean(col.Name, opts...)

	default:
		return mcp.WithString(col.Name)
	}
}
