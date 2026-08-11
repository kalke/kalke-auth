package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kalke/kalke-auth/internal/secrets"
)

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
	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) // #nosec G703 -- path restricted to /tmp/kalke-secrets.env
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	for k, v := range data {
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
	if cleaned != "/tmp/kalke-secrets.env" {
		return "", fmt.Errorf("outfile must be /tmp/kalke-secrets.env")
	}
	return cleaned, nil
}
