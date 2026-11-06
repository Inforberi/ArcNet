package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/arcvpn/arcvpn/internal/app"
	"github.com/arcvpn/arcvpn/internal/nodeagent"
	"github.com/arcvpn/arcvpn/internal/nodeprobe"
	"github.com/arcvpn/arcvpn/internal/store"
)

func runCascadeProbe(args []string) error {
	flags := flag.NewFlagSet("cascade-probe", flag.ContinueOnError)
	path := flags.String("grant-file", "", "private one-time probe file from the panel")
	expected := flags.String("controller", "", "independently confirmed HTTPS panel origin")
	binary := flags.String("xray", "xray", "trusted installed Xray executable")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if flags.NArg() != 0 || *path == "" || *expected == "" {
		return errors.New("requires --grant-file and --controller; keep credentials out of argv")
	}
	origin, e := nodeagent.ValidateOrigin(*expected)
	if e != nil {
		return e
	}
	raw, e := readProtectedFile(*path, 8192)
	if e != nil {
		return e
	}
	var bundle app.CascadeProbeBundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bundle) != nil || bundle.Protocol != 1 || bundle.Controller != origin || bundle.ExpiresAt < time.Now().Unix() || len(bundle.Token) < 40 || len(bundle.Token) > 128 {
		return errors.New("invalid, expired or different-controller cascade grant")
	}
	if _, e := decoder.Token(); e != io.EOF {
		return errors.New("trailing probe grant data")
	}
	start := time.Now()
	result, probeErr := nodeprobe.RunCascade(context.Background(), *binary, bundle.Link)
	report := store.CascadeProbeReport{DeploymentID: bundle.DeploymentID, Version: bundle.Version, EntryRevision: bundle.EntryRevision, EntryDigest: bundle.EntryDigest, ExitRevision: bundle.ExitRevision, ExitDigest: bundle.ExitDigest, Result: "fail", DurationMS: time.Since(start).Milliseconds()}
	if probeErr == nil {
		report.Result = result.Result
		report.ExitIP = result.ExitIP
		report.DNSMode = result.DNSMode
	}
	payload, _ := json.Marshal(report)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	request, e := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/api/cascade-probes/report", bytes.NewReader(payload))
	if e != nil {
		return e
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "CascadeProbe "+bundle.Token)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(request)
	if e != nil {
		return errors.New("could not report cascade result over verified HTTPS")
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("cascade report rejected: HTTP %d", response.StatusCode)
	}
	if probeErr != nil {
		return errors.New("external cascade probe failed; failure was reported")
	}
	if result.ExitIP != bundle.ExpectedIP {
		return errors.New("egress differs from expected exit; failure recorded by controller")
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
