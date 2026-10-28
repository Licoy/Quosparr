package app

import (
	"errors"
	"fmt"
	"github.com/licoy/quosparr/internal/mcp"
)

// mcpAddError deliberately never renders an unknown error's text.
func (u *settingsUI) mcpAddError(err error) string {
	if err == nil {
		return ""
	}
	var d *mcp.DraftError
	if errors.As(err, &d) {
		message := tx(u.L(), mcpErrorTextKey(d.Code))
		if field := u.mcpErrorField(d.Field); field != "" {
			message += " · " + field
		}
		if d.Line > 0 {
			message += " · " + tx(u.L(), xMCPJSONPosition, d.Line, d.Column)
		}
		return message
	}
	var p *mcp.ProbeError
	if errors.As(err, &p) {
		message := tx(u.L(), mcpErrorTextKey(p.Code))
		stages := map[string]textKey{"connect": xMCPErrorStageConnect, "discover": xMCPErrorStageDiscover, "initialize": xMCPErrorStageInitialize, "initialized": xMCPErrorStageInitialize, "tools": xMCPErrorStageTools}
		if stage, ok := stages[p.Stage]; ok {
			message += " · " + tx(u.L(), stage)
		}
		if p.HTTPStatus != 0 {
			message += fmt.Sprintf(" · HTTP %d", p.HTTPStatus)
		}
		if p.ExitCode != 0 {
			message += fmt.Sprintf(" · exit %d", p.ExitCode)
		}
		if p.RPCCode != 0 {
			message += fmt.Sprintf(" · RPC %d", p.RPCCode)
		}
		return message
	}
	return tx(u.L(), xMCPAddFailed)
}
func mcpErrorTextKey(code string) textKey {
	switch code {
	case "syntax":
		return xMCPErrorSyntax
	case "size", "depth", "response_too_large":
		return xMCPErrorSize
	case "duplicate":
		return xMCPErrorDuplicate
	case "conflict":
		return xMCPErrorConflict
	case "unsupported":
		return xMCPErrorUnsupported
	case "object", "type", "invalid", "invalid_draft":
		return xMCPErrorInvalid
	case "missing_env":
		return xMCPErrorEnv
	case "network":
		return xMCPErrorNetwork
	case "http_status":
		return xMCPErrorHTTP
	case "invalid_response":
		return xMCPErrorResponse
	case "protocol":
		return xMCPErrorProtocol
	case "rpc":
		return xMCPErrorRPC
	case "timeout":
		return xMCPErrorTimeout
	case "cancelled":
		return xMCPErrorCancelled
	case "process_start", "process_exit":
		return xMCPErrorProcess
	case "changed":
		return xMCPErrorChanged
	case "read", "write", "verification", "backup", "permissions", "unsafe", "encode":
		return xMCPErrorStorage
	}
	return xMCPAddFailed
}

// mcpErrorField names the field an error is about, or nothing for one the
// page has no name for.
func (u *settingsUI) mcpErrorField(field string) string {
	fields := map[string]textKey{"json": xMCPJSON, "name": xMCPName, "transport": xMCPTransport, "type": xMCPTransport, "url": xMCPURL,
		"command": xMCPCommand, "args": xMCPArgs, "env": xMCPEnv, "headers": xMCPHeaders, "http_headers": xMCPHeaders, "cwd": xMCPCwd,
		"env_headers": xMCPEnvHeaders, "env_http_headers": xMCPEnvHeaders, "bearer_token_env": xMCPBearerEnv, "bearer_token_env_var": xMCPBearerEnv,
		"disabled_mcp_servers": xMCPDisabledList}
	if key, ok := fields[field]; ok {
		return tx(u.L(), key)
	}
	return ""
}

func mcpWriteNeedsReview(err error) bool {
	var d *mcp.DraftError
	return errors.As(err, &d) && d.Code == "verification" && d.Field == "written_config"
}
