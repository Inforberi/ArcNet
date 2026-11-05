package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/arcvpn/arcvpn/internal/nodeagent"
	"github.com/arcvpn/arcvpn/internal/nodeprobe"
	"github.com/arcvpn/arcvpn/internal/store"
)

// runNodeProbe is intended to run on an independent Linux host. It neither
// initializes the controller DB nor needs root permissions. Client identities
// and report grants are read from protected files/stdin, never argv.
func runNodeProbe(args []string) error {
	flags := flag.NewFlagSet("node-probe", flag.ContinueOnError)
	linkFile := flags.String("link-file", "", "private file containing an ArcVPN VLESS link")
	binary := flags.String("xray", "xray", "trusted, independently obtained Xray executable")
	controller := flags.String("controller", "", "optional HTTPS controller origin for report")
	nodeID := flags.String("node-id", "", "node identifier for report")
	revision := flags.Int64("revision", 0, "applied revision for report")
	digest := flags.String("digest", "", "applied configuration SHA-256 for report")
	tokenStdin := flags.Bool("report-token-stdin", false, "read one-time report grant from stdin")
	tokenFile := flags.String("report-token-file", "", "read one-time report grant from a mode-0600 file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(flags.Args()) != 0 || *linkFile == "" {
		return errors.New("--link-file is required and positional arguments are not accepted")
	}
	raw, err := readProtectedFile(*linkFile, 2048)
	if err != nil {
		return err
	}
	link := strings.TrimSpace(string(raw))
	if _, err = nodeprobe.ParseLink(link); err != nil {
		return err
	}
	reporting := *controller != "" || *nodeID != "" || *revision != 0 || *digest != "" || *tokenStdin || *tokenFile != ""
	if reporting && (*controller == "" || *nodeID == "" || *revision <= 0 || len(*digest) != 64 || (*tokenStdin == (*tokenFile != ""))) {
		return errors.New("report requires --controller, --node-id, --revision, --digest and exactly one report token source")
	}
	var reportToken string
	if reporting {
		if _, err = nodeagent.ValidateOrigin(*controller); err != nil {
			return err
		}
		if *tokenStdin {
			input, readErr := bufio.NewReader(io.LimitReader(os.Stdin, 512)).ReadString('\n')
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return errors.New("cannot read report token from stdin")
			}
			reportToken = strings.TrimSpace(input)
		} else {
			input, readErr := readProtectedFile(*tokenFile, 512)
			if readErr != nil {
				return readErr
			}
			reportToken = strings.TrimSpace(string(input))
		}
		if len(reportToken) < 40 || len(reportToken) > 128 || strings.ContainsAny(reportToken, " \r\n\t") {
			return errors.New("invalid report token")
		}
	}
	start := time.Now()
	result, probeErr := nodeprobe.Run(context.Background(), *binary, link)
	if probeErr != nil {
		result = nodeprobe.Result{Result: "fail", CheckedAt: time.Now().Unix(), DurationMS: time.Since(start).Milliseconds(), Evidence: "external Xray client did not complete an authenticated HTTPS request"}
	}
	if reporting {
		report := store.ProbeReport{NodeID: *nodeID, Revision: *revision, Digest: *digest, Result: result.Result, DurationMS: result.DurationMS}
		if err = postNodeProbe(context.Background(), *controller, reportToken, report); err != nil {
			return err
		}
	}
	if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return err
	}
	return probeErr
}

func readProtectedFile(path string, limit int64) ([]byte, error) {
	// Open with O_NOFOLLOW before checking permissions. Doing Lstat then
	// OpenFile would leave a symlink-swap race between the two operations.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("cannot open private input file without following symlinks")
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > limit {
		return nil, errors.New("probe input must be a private regular file (0600), within size limit")
	}
	b, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("cannot read protected probe input")
	}
	return b, nil
}

func postNodeProbe(ctx context.Context, controller, secret string, report store.ProbeReport) error {
	origin, err := nodeagent.ValidateOrigin(controller)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, origin+"/api/nodes/probe/report", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Probe "+secret)
	request.Header.Set("Content-Type", "application/json")
	response, err := nodeagent.Client().Do(request)
	if err != nil {
		return errors.New("external probe could not report over verified HTTPS")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("controller did not accept probe report: HTTP %d", response.StatusCode)
	}
	return nil
}
