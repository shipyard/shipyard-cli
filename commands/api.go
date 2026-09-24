package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/shipyard/shipyard-cli/pkg/client"
	"github.com/shipyard/shipyard-cli/pkg/display"
	"github.com/shipyard/shipyard-cli/pkg/requests"
	"github.com/shipyard/shipyard-cli/pkg/requests/uri"
	"github.com/spf13/cobra"
)

// maxRequestBytes caps --input bodies; larger bodies are refused, never truncated.
const maxRequestBytes = 1 << 20 // 1 MiB

var allowedAPIMethods = map[string]bool{
	http.MethodGet:    true,
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodPatch:  true,
	http.MethodDelete: true,
}

// NewAPICmd creates shipyard api — an authenticated HTTP helper for the public
// Shipyard REST API (same idea as `gh api`). CLI/scripts only; not registered as MCP.
func NewAPICmd(c client.Client) *cobra.Command {
	var (
		method         string
		inputFile      string
		includeSecrets bool
		fieldArgs      []string
	)

	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Make an authenticated request to the Shipyard REST API",
		Long: `Call the public Shipyard API with the configured token and org.

Path must start with /api/v1 or /api/v2. Absolute URLs and browser session routes
are rejected. The x-api-token header is injected automatically.

Examples:
  shipyard api /api/v1/environment
  shipyard api -X POST /api/v1/environment/$UUID/rebuild
  shipyard api -X PUT /api/v1/environment/$UUID/env-vars --input body.json
  echo '{"env_vars":[{"name":"DEBUG","value":"true"}]}' | shipyard api -X PUT /api/v1/environment/$UUID/env-vars --input -`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAPI(c, strings.ToUpper(method), args[0], inputFile, includeSecrets, fieldArgs)
		},
	}

	cmd.Flags().StringVarP(&method, "method", "X", http.MethodGet, "HTTP method (GET, POST, PUT, PATCH, DELETE)")
	cmd.Flags().StringVar(&inputFile, "input", "", "JSON body file path, or - for stdin")
	cmd.Flags().BoolVar(&includeSecrets, "include-secrets", false, "Do not redact bypass_token or kubeconfig credentials in the response")
	cmd.Flags().StringArrayVarP(&fieldArgs, "field", "f", nil, "Add a string field to the JSON body as key=value (repeatable)")

	return cmd
}

func runAPI(c client.Client, method, path, inputFile string, includeSecrets bool, fieldArgs []string) error {
	if !allowedAPIMethods[method] {
		return fmt.Errorf("method %q is not allowed; use GET, POST, PUT, PATCH, or DELETE", method)
	}

	org := ""
	if c.OrgLookupFn != nil {
		org = c.OrgLookupFn()
	}
	requestURI, err := uri.ResolveAPIPath(path, org)
	if err != nil {
		return err
	}

	var body any
	if inputFile != "" || len(fieldArgs) > 0 {
		payload, err := buildAPIBody(inputFile, fieldArgs)
		if err != nil {
			return err
		}
		body = payload
	}

	resp, err := c.Requester.Do(method, requestURI, "application/json", body)
	if err != nil {
		// Non-2xx bodies come back inside the error; they can carry secrets too.
		return errors.New(string(requests.RedactAPIResponse([]byte(err.Error()), includeSecrets)))
	}

	// Redact before truncating: a cut-off JSON body can't be parsed for redaction.
	resp = requests.RedactAPIResponse(resp, includeSecrets)
	if len(resp) > uri.MaxResponseBytes() {
		fmt.Fprintf(os.Stderr, "warning: response truncated from %d to %d bytes\n", len(resp), uri.MaxResponseBytes())
		resp = resp[:uri.MaxResponseBytes()]
	}

	if len(resp) == 0 {
		display.Println(fmt.Sprintf("%s %s → empty body", method, path))
		return nil
	}
	display.Println(string(resp))
	return nil
}

func buildAPIBody(inputFile string, fieldArgs []string) ([]byte, error) {
	fields := make(map[string]any)
	for _, f := range fieldArgs {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --field %q; expected key=value", f)
		}
		fields[k] = v
	}

	if inputFile == "" {
		return marshalFields(fields)
	}

	var r io.Reader
	if inputFile == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(inputFile)
		if err != nil {
			return nil, fmt.Errorf("open --input: %w", err)
		}
		defer f.Close()
		r = f
	}

	raw, err := io.ReadAll(io.LimitReader(r, maxRequestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read --input: %w", err)
	}
	if len(raw) > maxRequestBytes {
		return nil, fmt.Errorf("--input is larger than %d bytes", maxRequestBytes)
	}
	if len(fields) == 0 {
		return raw, nil
	}

	// Merge -f fields into a JSON object body.
	obj := make(map[string]any)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("parse --input JSON object: %w", err)
		}
	}
	for k, v := range fields {
		obj[k] = v
	}
	return marshalFields(obj)
}

func marshalFields(fields map[string]any) ([]byte, error) {
	if len(fields) == 0 {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode body: %w", err)
	}
	return b, nil
}
