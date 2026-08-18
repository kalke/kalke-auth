package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kalke/kalke-auth/internal/secrets"
)

const secretsOutPath = "/tmp/kalke-secrets.env" // #nosec G101 -- fixed outfile path, not a credential

// loadsecret fetches a JSON Secrets Manager blob and writes KEY=value lines
// suitable for: set -a; . /path/to/file; set +a
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: loadsecret <outfile> [secret-id]")
		os.Exit(2)
	}
	outPath, err := safeOutPath(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	sid := ""
	if len(os.Args) >= 3 {
		sid = strings.TrimSpace(os.Args[2])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	data, err := secrets.FetchMap(ctx, sid)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	f, err := openSecretsFile(outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	// Do not overwrite Compose-pinned values (e.g. local Docker DATABASE_URL).
	for k, v := range secrets.WithoutExisting(data) {
		val := strings.ReplaceAll(v, `'`, `'"'"'`)
		fmt.Fprintf(w, "%s='%s'\n", k, val)
	}
	// Sentinel so the API process skips a second GetSecretValue.
	fmt.Fprintf(w, "%s='1'\n", "KALKE_SECRETS_LOADED")
	if err := w.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func safeOutPath(raw string) (string, error) {
	cleaned := filepath.Clean(raw)
	if cleaned != secretsOutPath {
		return "", fmt.Errorf("outfile must be %s", secretsOutPath)
	}
	if fi, err := os.Lstat(cleaned); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("outfile must not be a symlink")
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return cleaned, nil
}

func openSecretsFile(path string) (*os.File, error) {
	// O_NOFOLLOW refuses to open through a symlink (closes TOCTOU after Lstat).
	fd, err := syscall.Open(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
