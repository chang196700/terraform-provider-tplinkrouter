package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chang196700/terraform-provider-tplinkrouter/internal/catalog"
	"github.com/chang196700/terraform-provider-tplinkrouter/internal/client"
)

type auditReport struct {
	Version       int           `json:"version"`
	GeneratedAt   string        `json:"generated_at"`
	ReadOnly      bool          `json:"read_only"`
	Endpoint      string        `json:"endpoint"`
	Model         string        `json:"model"`
	Hardware      string        `json:"hardware_version"`
	Firmware      string        `json:"firmware_version"`
	Capabilities  []string      `json:"capabilities"`
	Results       []auditResult `json:"results"`
	Configuration string        `json:"configuration_status"`
}

type auditResult struct {
	Name          string       `json:"name"`
	Module        string       `json:"module"`
	Table         string       `json:"table,omitempty"`
	Section       string       `json:"section,omitempty"`
	Mode          catalog.Mode `json:"mode"`
	Status        string       `json:"status"`
	Rows          int          `json:"rows"`
	Fields        []string     `json:"fields"`
	MissingFields []string     `json:"missing_expected_fields,omitempty"`
	ErrorKind     string       `json:"error_kind,omitempty"`
	ErrorCode     *int         `json:"error_code,omitempty"`
	SubCodes      []int        `json:"sub_error_codes,omitempty"`
}

func auditCommand(args []string) error {
	flags := flag.NewFlagSet("audit", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "https://192.168.1.1", "Router HTTP or HTTPS origin")
	envFile := flags.String("env-file", ".env", "Dotenv file with USERNAME and PASSWORD; empty skips the file")
	insecure := flags.Bool("insecure-skip-verify", false, "Explicitly disable TLS certificate verification")
	legacy := flags.Bool("allow-legacy-tls", false, "Enable the firmware's static-RSA TLS 1.2 AES-GCM suites")
	timeout := flags.Duration("timeout", 30*time.Second, "Per-request timeout")
	output := flags.String("out", ".cache/catalog-audit.json", "Audit report destination (contains no configuration values)")
	only := flags.String("only", "", "Optional comma-separated spec names")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("audit does not accept positional arguments")
	}
	specs, err := specsSorted()
	if err != nil {
		return err
	}
	selection := map[string]bool{}
	if *only != "" {
		for _, name := range strings.Split(*only, ",") {
			selection[strings.TrimSpace(name)] = true
		}
		for name := range selection {
			found := false
			for _, spec := range specs {
				found = found || spec.Name == name
			}
			if !found {
				return errors.New("-only contains a name not present in the catalog")
			}
		}
	}
	values := map[string]string{}
	if *envFile != "" {
		values, err = client.LoadEnvFile(*envFile)
		if err != nil {
			return err
		}
	}
	username, password := os.Getenv("TPLINK_ROUTER_USERNAME"), os.Getenv("TPLINK_ROUTER_PASSWORD")
	if username == "" {
		username = values["USERNAME"]
	}
	if password == "" {
		password = values["PASSWORD"]
	}
	c, err := client.New(client.Config{Endpoint: *endpoint, Username: username, Password: password, Insecure: *insecure, AllowLegacyTLS: *legacy, ReadOnly: true, Timeout: *timeout})
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	defer func() {
		logoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Logout(logoutCtx)
	}()
	info, err := c.Query(ctx, map[string]any{"device_info": map[string]any{"name": "info"}, "function": map[string]any{"name": "new_module_spec"}, "system": map[string]any{"name": "cfg_status"}})
	if err != nil {
		return errors.New("read-only device discovery failed; verify endpoint, credentials, and TLS options")
	}
	device := nestedMap(info, "device_info", "info")
	features := nestedMap(info, "function", "new_module_spec")
	report := auditReport{Version: 1, GeneratedAt: time.Now().UTC().Format(time.RFC3339), ReadOnly: true, Endpoint: c.Endpoint(), Model: asString(device["device_model"]), Hardware: asString(device["hw_version"]), Firmware: asString(device["sw_version"]), Configuration: asString(nestedMap(info, "system", "cfg_status")["status"])}
	capabilities := map[string]bool{}
	if list, ok := features["new_module_list"].([]any); ok {
		for _, item := range list {
			name := asString(item)
			capabilities[name] = true
			report.Capabilities = append(report.Capabilities, name)
		}
	}
	sort.Strings(report.Capabilities)
	failures, successes, unavailable := 0, 0, 0
	for _, spec := range specs {
		if len(selection) > 0 && !selection[spec.Name] {
			continue
		}
		result := auditResult{Name: spec.Name, Module: spec.Module, Section: spec.Section, Table: spec.Table, Mode: spec.Mode, Fields: []string{}}
		err = nil
		if spec.Capability != "" && !capabilities[spec.Capability] {
			result.Status, result.ErrorKind = "UNAVAILABLE", "capability_not_exposed"
			unavailable++
		} else {
			operationCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			resolved := spec
			if spec.Resolve != nil {
				resolved, err = spec.Resolve(operationCtx, c, spec)
			}
			if err != nil {
				recordFailure(&result, "resolve_failed", err)
			} else {
				result.Module, result.Section, result.Table = resolved.Module, resolved.Section, resolved.Table
				var rows []map[string]any
				rows, err = auditRead(operationCtx, c, resolved)
				if err != nil {
					recordFailure(&result, "query_failed", err)
				} else {
					result.Status, result.Rows = "SUCCESS", len(rows)
					keys := map[string]bool{}
					for _, row := range rows {
						for key := range row {
							keys[strings.ReplaceAll(key, "\x1f", ".")] = true
						}
					}
					for key := range keys {
						result.Fields = append(result.Fields, key)
					}
					sort.Strings(result.Fields)
					if len(rows) > 0 {
						for _, field := range resolved.Fields {
							key := field.Wire
							if key == "" {
								key = field.Name
							}
							if field.Section != "" {
								key = field.Section + "." + key
							}
							if !field.WriteOnly && !keys[key] {
								result.MissingFields = append(result.MissingFields, key)
							}
						}
						sort.Strings(result.MissingFields)
					}
				}
			}
			cancel()
			if result.Status == "SUCCESS" {
				successes++
			} else {
				failures++
			}
		}
		report.Results = append(report.Results, result)
		fmt.Printf("%s %s rows=%d fields=%d", result.Status, result.Name, result.Rows, len(result.Fields))
		if result.ErrorCode != nil {
			fmt.Printf(" error_code=%d", *result.ErrorCode)
		}
		fmt.Println()
		// Write progress as sanitized facts so an interrupted audit remains useful.
		if err := writeAuditReport(*output, report); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	fmt.Printf("Read-only audit: %d successful, %d unavailable, %d failed; report %s\n", successes, unavailable, failures, *output)
	if failures > 0 {
		return errors.New("some catalog read queries failed; inspect numeric error codes in the sanitized report")
	}
	return nil
}

func recordFailure(result *auditResult, stage string, err error) {
	result.Status, result.ErrorKind = "ERROR", stage
	var apiErr *client.APIError
	var transportErr *client.TransportError
	if errors.As(err, &apiErr) {
		code := apiErr.Code
		result.ErrorCode, result.SubCodes = &code, apiErr.SubCodes
	} else if errors.As(err, &transportErr) {
		result.ErrorKind = "transport_" + transportErr.Kind
	} else if errors.Is(err, context.DeadlineExceeded) {
		result.ErrorKind = "timeout"
	} else if errors.Is(err, context.Canceled) {
		result.ErrorKind = "cancelled"
	} else {
		// These messages are created locally, never copied from router responses.
		message := strings.TrimPrefix(err.Error(), "router login failed: ")
		for _, known := range []string{"missing settings module", "missing settings section", "missing secondary settings section", "missing table module", "missing table", "table is not a list", "malformed table row", "missing stable table identity", "pagination did not advance", "table exceeded pagination limit", "router returned an invalid JSON response", "router returned trailing content after its JSON response", "router response is missing a valid error_code"} {
			if message == known {
				result.ErrorKind = strings.ReplaceAll(known, " ", "_")
			}
		}
		for _, known := range []string{"DNS response omitted dns module", "DNS response omitted global settings", "DNS global settings returned an unsupported shape"} {
			if message == known {
				result.ErrorKind = strings.ToLower(strings.ReplaceAll(known, " ", "_"))
			}
		}
		if code := strings.TrimPrefix(message, "router returned HTTP status "); code != message {
			if status, parseErr := strconv.Atoi(code); parseErr == nil {
				result.ErrorKind = fmt.Sprintf("http_status_%d", status)
			}
		}
	}
	// Adapter error strings may include values; intentionally do not store them.
}

func writeAuditReport(path string, report auditReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.New("cannot create audit output directory")
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return errors.New("cannot encode audit report")
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0600); err != nil {
		return errors.New("cannot write audit report")
	}
	return nil
}

func auditRead(ctx context.Context, c *client.Client, spec catalog.Spec) ([]map[string]any, error) {
	if spec.ReadRows != nil {
		return spec.ReadRows(ctx, c, spec)
	}
	if spec.Mode == catalog.Settings {
		names, seen := []string{spec.Section}, map[string]bool{spec.Section: true}
		for _, field := range spec.Fields {
			if field.Section != "" && !seen[field.Section] {
				names = append(names, field.Section)
				seen[field.Section] = true
			}
		}
		var selector any = spec.Section
		if len(names) > 1 {
			selector = names
		}
		response, err := c.Query(ctx, map[string]any{spec.Module: merge(spec.Query, map[string]any{"name": selector})})
		if err != nil {
			return nil, err
		}
		module, ok := response[spec.Module].(map[string]any)
		if !ok {
			return nil, errors.New("missing settings module")
		}
		base, ok := module[spec.Section].(map[string]any)
		if !ok {
			return nil, errors.New("missing settings section")
		}
		row := merge(base, nil)
		for _, field := range spec.Fields {
			if field.Section != "" {
				section, ok := module[field.Section].(map[string]any)
				if !ok {
					return nil, errors.New("missing secondary settings section")
				}
				key := field.Wire
				if key == "" {
					key = field.Name
				}
				if value, present := section[key]; present {
					row[field.Section+"\x1f"+key] = value
				}
			}
		}
		return []map[string]any{row}, nil
	}
	query := merge(spec.Query, map[string]any{"table": spec.Table})
	if len(spec.Filter) > 0 {
		if _, present := query["filter"]; !present {
			query["filter"] = []any{spec.Filter}
		}
	}
	rows, seen := []map[string]any{}, map[string]bool{}
	for page := 0; page < 10000; page++ {
		response, err := c.Query(ctx, map[string]any{spec.Module: query})
		if err != nil {
			return nil, err
		}
		module, ok := response[spec.Module].(map[string]any)
		if !ok {
			return nil, errors.New("missing table module")
		}
		raw, present := module[spec.Table]
		if !present {
			return nil, errors.New("missing table")
		}
		items, ok := raw.([]any)
		if raw == nil {
			items, ok = []any{}, true
		}
		if !ok {
			return nil, errors.New("table is not a list")
		}
		added := 0
		for _, item := range items {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("malformed table row")
			}
			section := ""
			if len(row) == 1 {
				for key, value := range row {
					if inner, ok := value.(map[string]any); ok {
						row, section = inner, key
					}
				}
			}
			identity, valid := auditIdentity(spec, section, row)
			if !valid {
				if !spec.DataSourceOnly {
					return nil, errors.New("missing stable table identity")
				}
				encoded, _ := json.Marshal(row)
				identity = fmt.Sprintf("%x", sha256.Sum256(encoded))
			}
			if seen[identity] {
				continue
			}
			seen[identity] = true
			rows, added = append(rows, row), added+1
		}
		count := len(rows)
		if counts, ok := module["count"].(map[string]any); ok {
			if value, exists := counts[spec.Table]; exists {
				count, _ = strconv.Atoi(asString(value))
			}
		}
		if len(rows) >= count {
			return rows, nil
		}
		if added == 0 {
			return nil, errors.New("pagination did not advance")
		}
		para, _ := query["para"].(map[string]any)
		query["para"] = merge(para, map[string]any{"start": len(rows), "end": len(rows) + 99})
	}
	return nil, errors.New("table exceeded pagination limit")
}

func auditIdentity(spec catalog.Spec, section string, row map[string]any) (string, bool) {
	keys := spec.IDFields
	if len(keys) == 0 && spec.IDField != "" {
		keys = []string{spec.IDField}
	}
	if len(keys) > 0 {
		values := make([]string, len(keys))
		for i, key := range keys {
			value := row[key]
			if list, ok := value.([]any); ok && len(list) == 1 {
				value = list[0]
			}
			if list, ok := value.([]string); ok && len(list) == 1 {
				value = list[0]
			}
			values[i] = asString(value)
			if values[i] == "" {
				return "", false
			}
		}
		encoded, _ := json.Marshal(values)
		return string(encoded), true
	}
	if section != "" {
		return section, true
	}
	identity := asString(row[".name"])
	return identity, identity != ""
}

func merge(left, right map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range left {
		out[key] = value
	}
	for key, value := range right {
		out[key] = value
	}
	return out
}

func nestedMap(value map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		value, _ = value[key].(map[string]any)
	}
	return value
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

