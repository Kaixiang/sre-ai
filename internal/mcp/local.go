package mcp

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "sort"
    "strings"
    "time"
)

// RunLocalCommand executes a configured MCP server command with optional arguments and environment overrides.
func RunLocalCommand(ctx context.Context, alias string, extraArgs []string, stdin string, extraEnv map[string]string) (string, string, int, error) {
    def, err := GetLocalServer(alias)
    if err != nil {
        return "", "", 0, err
    }
    return runCommandWithDefinition(ctx, alias, def, extraArgs, stdin, extraEnv)
}

// TestLocalServer attempts to start the configured command and ensures it can be launched.
func TestLocalServer(ctx context.Context, alias string) error {
    def, err := GetLocalServer(alias)
    if err != nil {
        return err
    }
    if def.Command == "" {
        return errors.New("server command is empty")
    }

    cmd, _, stderr, err := buildCommand(ctx, alias, def, nil, "", nil)
    if err != nil {
        return err
    }

    if err := cmd.Start(); err != nil {
        return fmt.Errorf("failed to start %s: %w", alias, err)
    }

    done := make(chan error, 1)
    go func() { done <- cmd.Wait() }()

    select {
    case err := <-done:
        if err != nil {
            return fmt.Errorf("%s exited with error: %w\n%s", alias, err, tail(stderr.String(), 400))
        }
    case <-time.After(700 * time.Millisecond):
        if cmd.Process != nil {
            _ = cmd.Process.Kill()
            <-done
        }
    }

    return nil
}

func runCommandWithDefinition(ctx context.Context, alias string, def ServerDefinition, extraArgs []string, stdin string, extraEnv map[string]string) (string, string, int, error) {
    if def.Command == "" {
        return "", "", 0, errors.New("server command is empty")
    }

    cmd, stdout, stderr, err := buildCommand(ctx, alias, def, extraArgs, stdin, extraEnv)
    if err != nil {
        return "", "", 0, err
    }

    err = cmd.Run()
    exitCode := 0
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            exitCode = exitErr.ExitCode()
            return stdout.String(), stderr.String(), exitCode, fmt.Errorf("%s exited with %d: %s", alias, exitCode, tail(stderr.String(), 400))
        }
        return stdout.String(), stderr.String(), exitCode, fmt.Errorf("unable to execute %s: %w", alias, err)
    }

    if cmd.ProcessState != nil {
        exitCode = cmd.ProcessState.ExitCode()
    }

    return stdout.String(), stderr.String(), exitCode, nil
}

func buildCommand(ctx context.Context, alias string, def ServerDefinition, extraArgs []string, stdin string, extraEnv map[string]string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer, error) {
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

    cmd := exec.CommandContext(ctx, def.Command, args...)
    if def.Workdir != "" {
        cmd.Dir = def.Workdir
    }
    cmd.Env = mergeEnv(envMap)

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
