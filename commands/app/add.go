// Package app holds the commands that set up Shipyard apps.
package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"unicode"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/shipyard/shipyard-cli/pkg/apps"
	"github.com/shipyard/shipyard-cli/pkg/client"
)

// errStopped is returned when the user declines or core needs an action first;
// the reason has already been printed.
var errStopped = errors.New("no app was created")

// mainRoute is the route the user picks a service for.
const mainRoute = "/"

func NewAddCmd(c client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a Shipyard app for the current repo and branch",
		Long: `Creates a Shipyard app from the git checkout in the current directory: its
GitHub origin repo and current branch. The branch must be pushed, with no local
commits or uncommitted changes, so Shipyard builds exactly what you have.

It shows what Shipyard found (services, routes, env var names) and asks before
creating anything. Env var values are asked for only for the names you choose,
and are never printed.`,
		Example: `  # From a clone of a GitHub repo, on a pushed branch
  shipyard add`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			a := &adder{
				client:     c,
				git:        runGit,
				in:         bufio.NewReader(cmd.InOrStdin()),
				out:        cmd.OutOrStdout(),
				readSecret: secretReader(cmd.InOrStdin()),
			}
			return cleanError(a.run())
		},
	}
	return cmd
}

// cleanError strips control characters from an error before cobra prints it:
// an API error can quote compose-file content. errStopped is kept as is.
func cleanError(err error) error {
	if err == nil || errors.Is(err, errStopped) {
		return err
	}
	return errors.New(withoutControl(err.Error()))
}

// adder runs one `shipyard add`. Its dependencies are fields so tests can
// stand in for git, the terminal and the API.
type adder struct {
	client client.Client
	git    gitRunner
	in     *bufio.Reader
	out    io.Writer
	// readSecret reads a value without echoing it when stdin is a terminal.
	readSecret func(in *bufio.Reader) (string, error)
}

func secretReader(stdin io.Reader) func(in *bufio.Reader) (string, error) {
	return func(in *bufio.Reader) (string, error) {
		if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
			return readPassword(int(f.Fd()))
		}
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
}

// readPassword reads a line without echo. Ctrl-C would otherwise end the
// process with echo still off, so it restores the terminal before exiting.
func readPassword(fd int) (string, error) {
	state, err := term.GetState(fd)
	if err != nil {
		return "", err
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	defer func() {
		signal.Stop(sigs)
		close(done)
	}()
	go func() {
		select {
		case <-sigs:
			_ = term.Restore(fd, state)
			os.Exit(130)
		case <-done:
		}
	}()
	b, err := term.ReadPassword(fd)
	return string(b), err
}

// printf writes to the terminal. String arguments come from the repo or the
// server (service names, routes, messages), so their control characters are
// replaced: an escape sequence in a compose file must not rewrite the screen.
func (a *adder) printf(format string, args ...any) {
	for i, arg := range args {
		if s, ok := arg.(string); ok {
			args[i] = withoutControl(s)
		}
	}
	_, _ = fmt.Fprintf(a.out, format, args...)
}

// withoutControl replaces control characters other than newline and tab.
func withoutControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
}

func (a *adder) run() error {
	co, err := preflight(a.git)
	if err != nil {
		return err
	}

	a.printf("Planning an app for %s on branch %s (%s)...\n", co.Repo, co.Branch, short(co.HeadSHA))
	body, err := apps.Plan(a.client, co.Repo, co.Branch)
	if actionBody, ok := apps.ActionBody(err); ok {
		a.printAction(actionBody, nil, nil)
		return errStopped
	}
	if err != nil {
		return err
	}
	var plan apps.PlanResponse
	if err := json.Unmarshal(body, &plan); err != nil {
		return fmt.Errorf("reading the plan: %w", err)
	}
	// A plan can answer creating or already_exists too: stop before any prompt
	if apps.IsActionBody(body) {
		a.printAction(body, nil, nil)
		return errStopped
	}
	// Core plans the branch head it reads from GitHub. If that is not the
	// commit preflight checked, stop rather than build a commit the user does
	// not have. GitHub's API can lag a fresh push, so the head may be older.
	if plan.HeadSHA != "" && plan.HeadSHA != co.HeadSHA {
		return fmt.Errorf("branch %s on GitHub is at %s but your checkout is at %s. If you just pushed, wait a few seconds "+
			"and run shipyard add again; otherwise fetch and reconcile with origin",
			co.Branch, short(plan.HeadSHA), short(co.HeadSHA))
	}

	org := a.orgName(plan)
	a.printPlan(plan, org)

	name, err := a.ask("App name", repoName(co.Repo))
	if err != nil {
		return err
	}
	services, err := a.chooseServices(plan.Services)
	if err != nil {
		return err
	}
	routes, err := a.chooseRoutes(plan, services)
	if err != nil {
		return err
	}
	env, err := a.chooseEnv(plan.Env)
	if err != nil {
		return err
	}

	a.printf("\n")
	ok, err := a.confirm(fmt.Sprintf("Create app %q in org %s from %s@%s with services %s?", name, org, co.Repo, co.Branch, orNone(services)), true)
	if err != nil {
		return err
	}
	if !ok {
		a.printf("Nothing was created.\n")
		return nil
	}

	project := apps.Project{
		Repo:            co.Repo,
		Branch:          co.Branch,
		Services:        services,
		Routes:          routes,
		ComposeFilename: plan.ComposeFile,
	}
	payload, err := apps.CreatePayload(apps.CreateRequest{
		AppName: name,
		Project: project,
		Env:     env,
		HeadSHA: co.HeadSHA,
	})
	if err != nil {
		return err
	}

	public := apps.PublicValues(name, project)
	body, err = apps.Create(a.client, payload)
	if actionBody, ok := apps.ActionBody(err); ok {
		a.printAction(actionBody, env, public)
		return errStopped
	}
	if err != nil {
		return apps.RedactError(err, env, public...)
	}
	if apps.IsActionBody(body) {
		a.printAction(body, env, public)
		return errStopped
	}
	a.printCreated(body)
	return nil
}

func repoName(repo string) string {
	_, name, _ := apps.SplitRepo(repo)
	return name
}

// orgName is the org the app will be created in: the one core planned for, or
// else the one this CLI sends.
func (a *adder) orgName(plan apps.PlanResponse) string {
	if plan.Org != "" {
		return plan.Org
	}
	if a.client.OrgLookupFn != nil {
		if org := a.client.OrgLookupFn(); org != "" {
			return org
		}
	}
	return "(your default org)"
}

func (a *adder) printPlan(plan apps.PlanResponse, org string) {
	a.printf("Org: %s\n", org)
	if plan.ComposeFile != "" {
		a.printf("Compose file: %s\n", plan.ComposeFile)
	}
	var services []string
	for _, s := range plan.Services {
		if s.EnabledByDefault {
			services = append(services, s.Name)
		} else {
			services = append(services, s.Name+" (off by default)")
		}
	}
	a.printf("Services: %s\n", orNone(services))
	if len(plan.Routes) > 0 {
		a.printf("Routes:\n")
		for _, r := range plan.Routes {
			a.printf("  %s -> %s\n", r.Route, r.Service)
		}
	}
	if len(plan.Env) > 0 {
		a.printf("Env vars:\n")
		for _, e := range plan.Env {
			a.printf("  %s (%s)\n", e.Name, kindLabel(e.Kind))
		}
	}
	if len(plan.ExistingApp) > 0 && string(plan.ExistingApp) != "null" {
		a.printf("An app already exists for this repo: %s\n", compactJSON(plan.ExistingApp))
	}
}

func kindLabel(kind string) string {
	if kind == apps.KindBuildArg {
		return "build arg"
	}
	return "runtime"
}

func orNone(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}

// chooseServices offers the services core enables by default, those not behind
// a compose profile, and lets the user pick another set.
func (a *adder) chooseServices(plan []apps.PlanService) ([]string, error) {
	if len(plan) == 0 {
		return nil, nil
	}
	var planned, defaults []string
	for _, s := range plan {
		planned = append(planned, s.Name)
		if s.EnabledByDefault {
			defaults = append(defaults, s.Name)
		}
	}
	if len(defaults) > 0 {
		ok, err := a.confirm(fmt.Sprintf("Enable %s?", strings.Join(defaults, ", ")), true)
		if err != nil || ok {
			return defaults, err
		}
	}
	for {
		answer, err := a.ask("Services to enable, comma-separated", strings.Join(defaults, ","))
		if err != nil {
			return nil, err
		}
		var chosen []string
		var unknown []string
		for _, s := range strings.Split(answer, ",") {
			s = strings.TrimSpace(s)
			switch {
			case s == "" || slices.Contains(chosen, s):
			case slices.Contains(planned, s):
				chosen = append(chosen, s)
			default:
				unknown = append(unknown, s)
			}
		}
		if len(unknown) > 0 {
			a.printf("Not in the compose file: %s. Choose from %s\n", strings.Join(unknown, ", "), strings.Join(planned, ", "))
			continue
		}
		if len(chosen) == 0 {
			a.printf("Choose at least one service\n")
			continue
		}
		return chosen, nil
	}
}

// chooseRoutes keeps the plan's routes for enabled services and, when core
// offered route candidates, lets the user pick which one serves the main route.
func (a *adder) chooseRoutes(plan apps.PlanResponse, services []string) (map[string]string, error) {
	routes := make(map[string]string)
	current := ""
	for _, r := range plan.Routes {
		if r.Service == "" || r.Route == "" || (services != nil && !slices.Contains(services, r.Service)) {
			continue
		}
		routes[r.Service] = r.Route
		if r.Route == mainRoute {
			current = r.Service
		}
	}

	var candidates []string
	for _, c := range plan.RouteCandidates {
		if s := string(c); s != "" && (services == nil || slices.Contains(services, s)) {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) == 0 {
		return routes, nil
	}
	if current == "" {
		current = candidates[0]
	}
	for {
		answer, err := a.ask(fmt.Sprintf("Service for the %s route (%s)", mainRoute, strings.Join(candidates, ", ")), current)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(candidates, answer) {
			a.printf("Choose one of %s\n", strings.Join(candidates, ", "))
			continue
		}
		for svc, route := range routes {
			if route == mainRoute && svc != answer {
				delete(routes, svc)
			}
		}
		routes[answer] = mainRoute
		return routes, nil
	}
}

// chooseEnv asks, name by name, which env vars to set now and reads their
// values. Values are never echoed or printed.
func (a *adder) chooseEnv(names []apps.EnvName) ([]apps.EnvItem, error) {
	var items []apps.EnvItem
	for _, e := range names {
		ok, err := a.confirm(fmt.Sprintf("Set %s (%s) now?", e.Name, kindLabel(e.Kind)), false)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		// An empty value would clear the variable; answer n above to skip it
		var value string
		for value == "" {
			a.printf("Value for %s (not shown): ", e.Name)
			value, err = a.readSecret(a.in)
			a.printf("\n")
			if err != nil {
				return nil, fmt.Errorf("reading the value of %s: %w", e.Name, err)
			}
			if value == "" {
				a.printf("A value is required.\n")
			}
		}
		kind := e.Kind
		if kind == "" {
			kind = apps.KindRuntime
		}
		items = append(items, apps.EnvItem{Name: e.Name, Value: value, Kind: kind})
	}
	return items, nil
}

func (a *adder) ask(prompt, def string) (string, error) {
	if def != "" {
		a.printf("%s [%s]: ", prompt, def)
	} else {
		a.printf("%s: ", prompt)
	}
	line, err := a.in.ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("no answer to %q: input ended", prompt)
		}
		return "", err
	}
	if answer := strings.TrimSpace(line); answer != "" {
		return answer, nil
	}
	return def, nil
}

func (a *adder) confirm(prompt string, def bool) (bool, error) {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	for {
		a.printf("%s [%s]: ", prompt, hint)
		line, err := a.in.ReadString('\n')
		if err != nil && line == "" {
			if errors.Is(err, io.EOF) {
				return false, fmt.Errorf("no answer to %q: input ended", prompt)
			}
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		a.printf("Answer y or n\n")
	}
}

// printAction shows a needs_action, already_exists or creating answer: what to
// do next and, for an existing app, everything else core sent about it.
func (a *adder) printAction(body []byte, env []apps.EnvItem, public []string) {
	body = apps.RedactValues(body, env, public...)
	var resp map[string]json.RawMessage
	_ = json.Unmarshal(body, &resp)
	var action apps.ActionResponse
	_ = json.Unmarshal(body, &action)

	switch action.Status {
	case apps.StatusAlreadyExists:
		a.printf("An app already exists for this repo and branch.\n")
		for _, key := range slices.Sorted(maps.Keys(resp)) {
			if key == "status" || key == "next_step" || key == "errors" {
				continue
			}
			a.printf("%s:\n%s\n", key, indentJSON(resp[key]))
		}
	case apps.StatusCreating:
		a.printf("This app is still being created by another request. Run this again in a minute to see it.\n")
	default:
		a.printf("Shipyard needs something done before it can create this app.\n")
	}
	if ns := action.NextStep; ns != nil {
		if ns.Message != "" {
			a.printf("Next step: %s\n", ns.Message)
		}
		if ns.URL != "" {
			a.printf("Open: %s\n", ns.URL)
		}
	}
}

func (a *adder) printCreated(body []byte) {
	var resp struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"attributes"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	a.printf("Created app %s (%s). Its first build is starting.\n", resp.Data.Attributes.Name, resp.Data.ID)
	if resp.Data.Attributes.URL != "" {
		a.printf("URL: %s\n", resp.Data.Attributes.URL)
	}
}

func indentJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "  " + string(raw)
	}
	b, err := json.MarshalIndent(v, "  ", "  ")
	if err != nil {
		return "  " + string(raw)
	}
	return "  " + string(b)
}

func compactJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
