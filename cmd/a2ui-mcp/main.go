package main

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type discoveryInput struct{}
type discoveryOutput struct{}

func newMCPServer(_ any) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "a2ui-mcp", Version: "p0"}, nil)
	notImplemented := func(context.Context, *mcp.CallToolRequest, discoveryInput) (*mcp.CallToolResult, discoveryOutput, error) {
		return nil, discoveryOutput{}, errors.New("A2UI MCP tool backend is not connected")
	}
	mcp.AddTool(server, &mcp.Tool{Name: "a2ui_publish", Description: "Publish A2UI V1 operations to the persistent daemon"}, notImplemented)
	mcp.AddTool(server, &mcp.Tool{Name: "a2ui_wait_event", Description: "Wait for semantic A2UI events from the persistent daemon"}, notImplemented)
	mcp.AddTool(server, &mcp.Tool{Name: "a2ui_status", Description: "Read persistent A2UI daemon status"}, notImplemented)
	return server
}
