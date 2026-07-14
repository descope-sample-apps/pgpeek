package server

import (
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxMCPResponseBytes          = 512 << 10
	maxMCPStructuredContentBytes = 448 << 10
)

var errMCPResultMetadataExceedsLimit = errors.New("tool result metadata exceeds MCP response limit")

func mcpStructuredOutput[Out any](message string, output Out, err error) (*mcp.CallToolResult, Out, error) {
	if err != nil {
		return nil, output, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: message}}}, output, nil
}

func capMCPListDatabasesOutput(output mcpListDatabasesOutput) (mcpListDatabasesOutput, error) {
	if mcpOutputFits(output) {
		return output, nil
	}
	count, err := maxMCPItemCount(len(output.Databases), func(count int) ([]byte, error) {
		candidate := output
		candidate.Databases = output.Databases[:count]
		candidate.Truncated = true
		return json.Marshal(candidate)
	})
	if err != nil {
		return mcpListDatabasesOutput{}, err
	}
	output.Databases = output.Databases[:count]
	output.Truncated = true
	return output, nil
}

func capMCPListTablesOutput(output mcpListTablesOutput) (mcpListTablesOutput, error) {
	if mcpOutputFits(output) {
		return output, nil
	}
	count, err := maxMCPItemCount(len(output.Tables), func(count int) ([]byte, error) {
		candidate := output
		candidate.Tables = output.Tables[:count]
		candidate.Truncated = true
		return json.Marshal(candidate)
	})
	if err != nil {
		return mcpListTablesOutput{}, err
	}
	output.Tables = output.Tables[:count]
	output.Truncated = true
	return output, nil
}

func capMCPDescribeTableOutput(output mcpDescribeTableOutput) (mcpDescribeTableOutput, error) {
	if mcpOutputFits(output) {
		return output, nil
	}
	columnTotal := len(output.Columns)
	count, err := maxMCPItemCount(columnTotal+len(output.ForeignKeys), func(count int) ([]byte, error) {
		candidate := output
		columnCount := min(count, columnTotal)
		foreignKeyCount := max(count-columnTotal, 0)
		candidate.Columns = output.Columns[:columnCount]
		candidate.ForeignKeys = output.ForeignKeys[:foreignKeyCount]
		candidate.Truncated = true
		return json.Marshal(candidate)
	})
	if err != nil {
		return mcpDescribeTableOutput{}, err
	}
	columnCount := min(count, columnTotal)
	foreignKeyCount := max(count-columnTotal, 0)
	output.Columns = output.Columns[:columnCount]
	output.ForeignKeys = output.ForeignKeys[:foreignKeyCount]
	output.Truncated = true
	return output, nil
}

func capMCPQueryOutput(output mcpQueryOutput) (mcpQueryOutput, error) {
	encoded, err := json.Marshal(output)
	if err == nil && len(encoded) <= maxMCPStructuredContentBytes {
		return output, nil
	}

	low, high := 0, len(output.Rows)
	for low < high {
		mid := low + (high-low+1)/2
		candidate := output
		candidate.Rows = output.Rows[:mid]
		candidate.RowCount = mid
		candidate.Truncated = true
		encoded, err = json.Marshal(candidate)
		if err == nil && len(encoded) <= maxMCPStructuredContentBytes {
			low = mid
		} else {
			high = mid - 1
		}
	}
	output.Rows = output.Rows[:low]
	output.RowCount = low
	output.Truncated = true
	encoded, err = json.Marshal(output)
	if err != nil || len(encoded) > maxMCPStructuredContentBytes {
		return mcpQueryOutput{}, errors.New("query result metadata exceeds MCP response limit")
	}
	return output, nil
}

func maxMCPItemCount(total int, encode func(int) ([]byte, error)) (int, error) {
	encoded, err := encode(0)
	if err != nil || len(encoded) > maxMCPStructuredContentBytes {
		return 0, errMCPResultMetadataExceedsLimit
	}
	low, high := 0, total
	for low < high {
		mid := low + (high-low+1)/2
		encoded, err = encode(mid)
		if err == nil && len(encoded) <= maxMCPStructuredContentBytes {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low, nil
}

func mcpOutputFits[Out any](output Out) bool {
	encoded, err := json.Marshal(output)
	return err == nil && len(encoded) <= maxMCPStructuredContentBytes
}
