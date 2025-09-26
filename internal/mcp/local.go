package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Logger interface {
	Printf(format string, v ...interface{})
}

// RunLocalCommand executes a configured MCP server command with optional arguments and environment overrides.
func RunLocalCommand(ctx context.Context, alias string, extraArgs []string, stdin string, extraEnv map[string]string, logger Logger) (string, string, int, error) {
	def, err := GetLocalServer(alias)
	if err != nil {
		return "", "", 0, err
	}
	return runCommandWithDefinition(ctx, alias, def, extraArgs, stdin, extraEnv, logger)
}

// TestLocalServer attempts to start the configured command and ensures it can be launched.
func TestLocalServer(ctx context.Context, alias string) error {
	_, err := ProbeLocalServer(ctx, alias)
	return err
}

// TestLocalServerWithLogger allows injecting a logger for verbose output.
func TestLocalServerWithLogger(ctx context.Context, alias string, logger Logger) error {
	_, err := ProbeLocalServerWithLogger(ctx, alias, logger)
	return err
}

// ToolSummary describes a tool exposed by a local MCP server.
type ToolSummary struct {
	Name        string                 `json:"name"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
	Annotations map[string]interface{} `json:"annotations,omitempty"`
}

// Notification captures a server notification observed during probing.
type Notification struct {
	Method string `json:"method"`
	Detail string `json:"detail,omitempty"`
}

// ProbeResult contains metadata collected from a probe run.
type ProbeResult struct {
	Alias           string                 `json:"alias"`
	ServerName      string                 `json:"serverName,omitempty"`
	ServerVersion   string                 `json:"serverVersion,omitempty"`
	ProtocolVersion string                 `json:"protocolVersion,omitempty"`
	Instructions    string                 `json:"instructions,omitempty"`
	Capabilities    map[string]interface{} `json:"capabilities,omitempty"`
	Tools           []ToolSummary          `json:"tools,omitempty"`
	Notifications   []Notification         `json:"notifications,omitempty"`
	Duration        time.Duration          `json:"duration"`
	Stderr          string                 `json:"stderr,omitempty"`
}

// ProbeLocalServer connects to a local MCP server using the stdio transport and reports its tooling.
func ProbeLocalServer(ctx context.Context, alias string) (*ProbeResult, error) {
	return ProbeLocalServerWithLogger(ctx, alias, nil)
}

// ProbeLocalServerWithLogger behaves like ProbeLocalServer but emits debug logging when logger is provided.
func ProbeLocalServerWithLogger(ctx context.Context, alias string, logger Logger) (*ProbeResult, error) {
	return probeLocalServer(ctx, alias, logger)
}

// jsonrpcEnvelope represents a JSON-RPC message exchanged with the MCP server.
type jsonrpcEnvelope struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *jsonrpcError    `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func probeLocalServer(ctx context.Context, alias string, logger Logger) (*ProbeResult, error) {
	start := time.Now()

	def, err := GetLocalServer(alias)
	if err != nil {
		return nil, err
	}
	if def.Command == "" {
		return nil, errors.New("server command is empty")
	}

	args := append([]string{}, def.Args...)
	envMap := map[string]string{}
	for k, v := range def.Env {
		envMap[k] = v
	}

	if logger != nil {
		logger.Printf("mcp probe alias=%s command=%s args=%s", alias, def.Command, strings.Join(args, " "))
		logger.Printf("mcp probe alias=%s env=%s", alias, debugMap(envMap))
		if def.Workdir != "" {
			logger.Printf("mcp probe alias=%s workdir=%s", alias, def.Workdir)
		}
	}

	cmd := exec.CommandContext(ctx, def.Command, args...)
	if def.Workdir != "" {
		cmd.Dir = def.Workdir
	}
	cmd.Env = mergeEnv(envMap)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", alias, err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	reader := bufio.NewReader(stdoutPipe)
	writer := bufio.NewWriter(stdinPipe)

	success := false
	defer func() {
		_ = writer.Flush()
		_ = stdinPipe.Close()
		wait := 200 * time.Millisecond
		if success {
			wait = 750 * time.Millisecond
		}
		select {
		case err := <-done:
			if err != nil && logger != nil && success {
				logger.Printf("mcp probe alias=%s exit error after probe: %v", alias, err)
			}
		case <-time.After(wait):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				<-done
			}
		}
	}()

	responses := make(map[string]jsonrpcEnvelope)
	notifications := make([]Notification, 0, 4)
	result := &ProbeResult{Alias: alias}

	requestID := 1
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2025-06-18",
			"clientInfo": map[string]string{
				"name":    "sre-ai",
				"version": "dev",
			},
			"capabilities": map[string]interface{}{},
		},
	}

	if err := sendJSONMessage(writer, initReq); err != nil {
		return nil, annotateProbeError(err, &stderr)
	}

	initEnv, err := awaitResponse(ctx, reader, writer, strconv.Itoa(requestID), responses, &notifications, done, alias, logger)
	if err != nil {
		return nil, annotateProbeError(err, &stderr)
	}
	if initEnv.Error != nil {
		return nil, annotateProbeError(fmt.Errorf("initialize failed: %s", initEnv.Error.Message), &stderr)
	}

	var initData struct {
		Capabilities    map[string]interface{} `json:"capabilities"`
		Instructions    string                 `json:"instructions"`
		ProtocolVersion string                 `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(initEnv.Result, &initData); err != nil {
		return nil, annotateProbeError(fmt.Errorf("decode initialize result: %w", err), &stderr)
	}

	result.Capabilities = initData.Capabilities
	result.Instructions = strings.TrimSpace(initData.Instructions)
	result.ProtocolVersion = initData.ProtocolVersion
	result.ServerName = initData.ServerInfo.Name
	result.ServerVersion = initData.ServerInfo.Version

	if err := sendJSONMessage(writer, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]interface{}{},
	}); err != nil {
		return nil, annotateProbeError(err, &stderr)
	}

	cursor := ""
	for {
		requestID++
		req := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      requestID,
			"method":  "tools/list",
		}
		if cursor != "" {
			req["params"] = map[string]interface{}{"cursor": cursor}
		}

		if err := sendJSONMessage(writer, req); err != nil {
			return nil, annotateProbeError(err, &stderr)
		}

		resp, err := awaitResponse(ctx, reader, writer, strconv.Itoa(requestID), responses, &notifications, done, alias, logger)
		if err != nil {
			return nil, annotateProbeError(err, &stderr)
		}
		if resp.Error != nil {
			return nil, annotateProbeError(fmt.Errorf("tools/list failed: %s", resp.Error.Message), &stderr)
		}

		var listResult struct {
			Tools      []map[string]interface{} `json:"tools"`
			NextCursor string                   `json:"nextCursor"`
		}
		if err := json.Unmarshal(resp.Result, &listResult); err != nil {
			return nil, annotateProbeError(fmt.Errorf("decode tools/list: %w", err), &stderr)
		}

		for _, tool := range listResult.Tools {
			summary := ToolSummary{}
			if name, ok := tool["name"].(string); ok {
				summary.Name = name
			}
			if title, ok := tool["title"].(string); ok {
				summary.Title = title
			}
			if desc, ok := tool["description"].(string); ok {
				summary.Description = desc
			}
			if annotations, ok := tool["annotations"].(map[string]interface{}); ok {
				summary.Annotations = annotations
				if summary.Title == "" {
					if title, ok := annotations["title"].(string); ok {
						summary.Title = title
					}
				}
			}
			if schema, ok := tool["inputSchema"].(map[string]interface{}); ok {
				summary.InputSchema = schema
			}
			result.Tools = append(result.Tools, summary)
		}

		if listResult.NextCursor == "" {
			break
		}
		cursor = listResult.NextCursor
	}

	result.Notifications = notifications
	result.Duration = time.Since(start)
	result.Stderr = strings.TrimSpace(stderr.String())

	success = true
	return result, nil
}

func sendJSONMessage(w *bufio.Writer, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := w.WriteString(header); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return w.Flush()
}

func awaitResponse(
	ctx context.Context,
	reader *bufio.Reader,
	writer *bufio.Writer,
	expectID string,
	pending map[string]jsonrpcEnvelope,
	notifications *[]Notification,
	done <-chan error,
	alias string,
	logger Logger,
) (jsonrpcEnvelope, error) {
	if env, ok := pending[expectID]; ok {
		delete(pending, expectID)
		return env, nil
	}

	for {
		select {
		case <-ctx.Done():
			return jsonrpcEnvelope{}, ctx.Err()
		case err := <-done:
			if err != nil {
				return jsonrpcEnvelope{}, fmt.Errorf("server exited: %w", err)
			}
			return jsonrpcEnvelope{}, errors.New("server exited before response")
		default:
		}

		msg, err := readFramedMessage(ctx, reader)
		if err != nil {
			return jsonrpcEnvelope{}, err
		}

		var env jsonrpcEnvelope
		if err := json.Unmarshal(msg, &env); err != nil {
			return jsonrpcEnvelope{}, fmt.Errorf("decode jsonrpc envelope: %w", err)
		}

		if env.ID != nil {
			id, err := rawMessageID(*env.ID)
			if err != nil {
				return jsonrpcEnvelope{}, err
			}
			if env.Method != "" {
				if logger != nil {
					logger.Printf("mcp probe alias=%s received request method=%s", alias, env.Method)
				}
				if err := respondMethodNotImplemented(writer, env); err != nil {
					return jsonrpcEnvelope{}, err
				}
				continue
			}
			if id == expectID {
				return env, nil
			}
			pending[id] = env
			continue
		}

		if env.Method != "" {
			detail := compactJSONRaw(env.Params)
			if len(detail) > 400 {
				detail = detail[:397] + "..."
			}
			*notifications = append(*notifications, Notification{Method: env.Method, Detail: detail})
			if logger != nil {
				logger.Printf("mcp probe alias=%s notify method=%s detail=%s", alias, env.Method, detail)
			}
			continue
		}
	}
}

func readFramedMessage(ctx context.Context, reader *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := readLineWithContext(ctx, reader)
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			if length >= 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			value := strings.TrimSpace(line[len("content-length:"):])
			l, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid content-length header: %s", line)
			}
			length = l
		}
	}
	if length < 0 {
		return nil, errors.New("missing Content-Length header")
	}
	buf := make([]byte, length)
	if err := readFullWithContext(ctx, reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func readLineWithContext(ctx context.Context, reader *bufio.Reader) (string, error) {
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := reader.ReadString('\n')
		ch <- result{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		return res.line, res.err
	}
}

func readFullWithContext(ctx context.Context, reader *bufio.Reader, buf []byte) error {
	type result struct {
		err error
	}
	ch := make(chan result, 1)
	go func() {
		_, err := io.ReadFull(reader, buf)
		ch <- result{err: err}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.err
	}
}

func rawMessageID(raw json.RawMessage) (string, error) {
	var intID int64
	if err := json.Unmarshal(raw, &intID); err == nil {
		return strconv.FormatInt(intID, 10), nil
	}
	var strID string
	if err := json.Unmarshal(raw, &strID); err == nil {
		return strID, nil
	}
	return "", fmt.Errorf("unsupported id type: %s", string(raw))
}

func respondMethodNotImplemented(writer *bufio.Writer, env jsonrpcEnvelope) error {
	var idValue interface{}
	if env.ID != nil {
		if err := json.Unmarshal(*env.ID, &idValue); err != nil {
			idValue = nil
		}
	}
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      idValue,
		"error": map[string]interface{}{
			"code":    -32601,
			"message": fmt.Sprintf("method %s not implemented in probe", env.Method),
		},
	}
	return sendJSONMessage(writer, payload)
}

func annotateProbeError(err error, stderr *bytes.Buffer) error {
	if err == nil {
		return nil
	}
	if stderr == nil {
		return err
	}
	if tail := strings.TrimSpace(stderr.String()); tail != "" {
		return fmt.Errorf("%w\nstderr: %s", err, tail)
	}
	return err
}

func compactJSONRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}
func runCommandWithDefinition(ctx context.Context, alias string, def ServerDefinition, extraArgs []string, stdin string, extraEnv map[string]string, logger Logger) (string, string, int, error) {
	if def.Command == "" {
		return "", "", 0, errors.New("server command is empty")
	}

	if logger != nil {
		logger.Printf("mcp run alias=%s command=%s baseArgs=%s extraArgs=%s", alias, def.Command, strings.Join(def.Args, " "), strings.Join(extraArgs, " "))
		if stdin != "" {
			logger.Printf("mcp run alias=%s stdin=%s", alias, maskValue(stdin))
		}
		if len(extraEnv) > 0 {
			logger.Printf("mcp run alias=%s extraEnv=%s", alias, debugMap(extraEnv))
		}
	}

	cmd, stdout, stderr, err := buildCommand(ctx, alias, def, extraArgs, stdin, extraEnv, logger)
	if err != nil {
		return "", "", 0, err
	}

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			if logger != nil {
				logger.Printf("mcp command error alias=%s exit=%d stderr=%s", alias, exitCode, tail(stderr.String(), 400))
			}
			return stdout.String(), stderr.String(), exitCode, fmt.Errorf("%s exited with %d: %s", alias, exitCode, tail(stderr.String(), 400))
		}
		if logger != nil {
			logger.Printf("mcp command failed alias=%s err=%v", alias, err)
		}
		return stdout.String(), stderr.String(), exitCode, fmt.Errorf("unable to execute %s: %w", alias, err)
	}

	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	if logger != nil {
		logger.Printf("mcp command success alias=%s exit=%d", alias, exitCode)
		if trimmed := strings.TrimSpace(stdout.String()); trimmed != "" {
			logger.Printf("mcp stdout alias=%s output=%s", alias, trimmed)
		}
		if trimmed := strings.TrimSpace(stderr.String()); trimmed != "" {
			logger.Printf("mcp stderr alias=%s output=%s", alias, trimmed)
		}
	}

	return stdout.String(), stderr.String(), exitCode, nil
}

func buildCommand(ctx context.Context, alias string, def ServerDefinition, extraArgs []string, stdin string, extraEnv map[string]string, logger Logger) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer, error) {
	args := append([]string{}, def.Args...)
	if len(extraArgs) > 0 {
		args = append(args, extraArgs...)
	}

	envMap := map[string]string{}
	for k, v := range def.Env {
		envMap[k] = v
	}
	for k, v := range extraEnv {
		envMap[k] = v
	}

	if logger != nil {
		logger.Printf("mcp build alias=%s command=%s args=%s", alias, def.Command, strings.Join(args, " "))
		if def.Workdir != "" {
			logger.Printf("mcp workdir alias=%s dir=%s", alias, def.Workdir)
		}
	}

	cmd := exec.CommandContext(ctx, def.Command, args...)
	if def.Workdir != "" {
		cmd.Dir = def.Workdir
	}
	mergedEnv := mergeEnv(envMap)
	if logger != nil {
		logger.Printf("mcp env alias=%s mergedKeys=%s", alias, envKeyList(mergedEnv))
	}
	cmd.Env = mergedEnv

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd, &stdout, &stderr, nil
}

func mergeEnv(custom map[string]string) []string {
	envMap := map[string]string{}
	for _, kv := range os.Environ() {
		if idx := strings.Index(kv, "="); idx != -1 {
			key := kv[:idx]
			value := kv[idx+1:]
			envMap[key] = value
		}
	}
	for k, v := range custom {
		envMap[k] = v
	}

	envMap["PATH"] = ensureNodeInPath(envMap["PATH"])

	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(env)
	return env
}

func ensureNodeInPath(existing string) string {
	nodeDirs := localNodeBinDirs()
	if len(nodeDirs) == 0 {
		return existing
	}

	sep := string(os.PathListSeparator)
	pieces := make([]string, 0, len(nodeDirs)+4)
	seen := map[string]struct{}{}

	for _, dir := range nodeDirs {
		if dir == "" {
			continue
		}
		key := strings.ToLower(dir)
		if _, ok := seen[key]; !ok {
			pieces = append(pieces, dir)
			seen[key] = struct{}{}
		}
	}

	if existing != "" {
		for _, part := range strings.Split(existing, sep) {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			pieces = append(pieces, trimmed)
		}
	}

	return strings.Join(pieces, sep)
}

func localNodeBinDirs() []string {
	exePath, err := os.Executable()
	if err != nil {
		return nil
	}
	exeDir := filepath.Dir(exePath)
	searchRoots := []string{
		filepath.Join(exeDir, "third_party", "node"),
		filepath.Join(exeDir, "..", "third_party", "node"),
	}

	var dirs []string
	for _, root := range searchRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(root, entry.Name())
			win := filepath.Join(candidate, "node.exe")
			unix := filepath.Join(candidate, "bin", "node")
			if fileExists(win) {
				dirs = append(dirs, candidate)
			} else if fileExists(unix) {
				dirs = append(dirs, filepath.Join(candidate, "bin"))
			}
		}
	}

	return dedupePaths(dirs)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dedupePaths(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		lp := strings.ToLower(p)
		if _, ok := seen[lp]; ok {
			continue
		}
		seen[lp] = struct{}{}
		out = append(out, p)
	}
	return out
}

func tail(input string, max int) string {
	if len(input) <= max {
		return strings.TrimSpace(input)
	}
	return strings.TrimSpace(input[len(input)-max:])
}

// LocalNodeExecutable returns the best guess for the bundled node binary.
func LocalNodeExecutable() (string, error) {
	dirs := localNodeBinDirs()
	for _, dir := range dirs {
		var candidate string
		if runtime.GOOS == "windows" {
			candidate = filepath.Join(dir, "node.exe")
			if fileExists(candidate) {
				return candidate, nil
			}
		} else {
			candidate = filepath.Join(dir, "node")
			if fileExists(candidate) {
				return candidate, nil
			}
		}
	}
	return "", errors.New("bundled node runtime not found")
}

func debugMap(values map[string]string) string {
	if len(values) == 0 {
		return "{}"
	}
	pairs := make([]string, 0, len(values))
	for k, v := range values {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func maskValue(input string) string {
	if input == "" {
		return ""
	}
	if len(input) <= 4 {
		return "***"
	}
	return input[:2] + ":" + input[len(input)-2:]
}

func envKeyList(entries []string) string {
	if len(entries) == 0 {
		return ""
	}
	keys := make([]string, 0, len(entries))
	for _, item := range entries {
		if idx := strings.Index(item, "="); idx != -1 {
			keys = append(keys, item[:idx])
		} else {
			keys = append(keys, item)
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
