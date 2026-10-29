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

// errStopped is returned when core answers needs_action, already_exists or
// creating; the reason has already been printed. Declining exits cleanly.
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
  shipyard add

  # With an org API token, name who may open the environment
  shipyard add --visitor octocat`,
		SilenceUsage: true,
	}
	var visitors []string
	cmd.Flags().StringArrayVar(&visitors, "visitor", nil,
		"GitHub login who may open the environment (repeatable); required with an org API token")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		a := &adder{
			client:     c,
			git:        runGit,
			in:         bufio.NewReader(cmd.InOrStdin()),
			out:        cmd.OutOrStdout(),
			readSecret: secretReader(cmd.InOrStdin()),
			visitors:   visitors,
		}
		return cleanError(a.run())
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
	// visitors are GitHub logins who may open the environment (--visitor).
	visitors []string
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
		case sig := <-sigs:
			_ = term.Restore(fd, state)
			// 128 + the signal number, as shells report it (130 for Ctrl-C)
			code := 130
			if sig == syscall.SIGTERM {
				code = 143
			}
			os.Exit(code)
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
			args[i] = oneLine(s)
		}
	}
	_, _ = fmt.Fprintf(a.out, format, args...)
}

// printBlock writes a multi-line block, such as indented JSON, keeping its
// newlines but replacing other control and format characters.
func (a *adder) printBlock(s string) {
	_, _ = io.WriteString(a.out, withoutControl(s))
}

// oneLine replaces control and format characters, newlines included: a name
// from the compose file must not break a prompt onto a forged line.
func oneLine(s string) string {
	return strings.ReplaceAll(withoutControl(s), "\n", "?")
}

// withoutControl replaces control and format characters other than newline
// and tab.
func withoutControl(s string) string {
	return strings.Map(func(r rune) rune {
		// Format characters (bidi overrides, zero-width) can reorder what is read
		if r != '\n' && r != '\t' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) {
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
	// Checked before decoding: an action answer need not fit a plan's fields,
	// and its next step must still be shown
	if apps.IsActionBody(body) {
		a.printAction(body, nil, nil)
		return errStopped
	}
	var plan apps.PlanResponse
	if err := json.Unmarshal(body, &plan); err != nil {
		return fmt.Errorf("reading the plan: %w", err)
	}
	// Core plans the branch head it reads from GitHub. If that is not the
	// commit preflight checked, stop rather than build a commit the user does
	// not have. GitHub's API can lag a fresh push, so the head may be older.
	if plan.HeadSHA != "" && plan.HeadSHA != co.HeadSHA {
		return fmt.Errorf("branch %s on GitHub is at %s but your checkout is at %s. If you just pushed, wait a few seconds "+
			"and run shipyard add again; otherwise fetch and reconcile with origin",
			co.Branch, short(plan.HeadSHA), short(co.HeadSHA))
	}

	// Core plans the repo under the name GitHub uses now; a renamed or
	// transferred repo still answers at the old name through a redirect
	if plan.Repo != "" && !strings.EqualFold(plan.Repo, co.Repo) {
		return fmt.Errorf("GitHub knows this repo as %s, but origin points at %s: update origin "+
			"(git remote set-url origin ...) and run shipyard add again", plan.Repo, co.Repo)
	}

	org := a.orgName(plan)
	a.printPlan(plan, org)
	// Ask before any value is typed when core would likely answer
	// already_exists and apply none of them. It replaces an earlier API create
	// that never got its first build, which the plan also shows, so the choice
	// is the user's.
	if len(plan.ExistingApp) > 0 && string(plan.ExistingApp) != "null" {
		a.printf("An app already exists for this repo and branch: %s\n", compactJSON(withoutBypassToken(plan.ExistingApp)))
		ok, err := a.confirm("Create anyway? Shipyard replaces it only if its first build never started", false)
		if err != nil {
			return err
		}
		if !ok {
			a.printf("Nothing was created.\n")
			return nil
		}
	}
	// Core rejects a create with no service
	if len(plan.Services) == 0 {
		return fmt.Errorf("the plan found no services in the compose file: add one, push, and run shipyard add again")
	}

	name, err := a.askAppName(apps.DefaultAppName(co.Repo, co.Branch), existingAppName(plan.ExistingApp))
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
	// No default: a blank line left over from a pasted multi-line value must
	// not create the app
	ok, err := a.confirmExplicit(fmt.Sprintf("Create app %q in org %s from %s@%s with services %s?", name, org, co.Repo, co.Branch, orNone(services)))
	if err != nil {
		return err
	}
	if !ok {
		a.printf("Nothing was created.\n")
		return nil
	}

	// No compose_filename: core pins one the caller names, and the plan's is
	// the one it detects anyway
	project := apps.Project{
		Repo:             co.Repo,
		Branch:           co.Branch,
		Services:         services,
		Routes:           routes,
		SelectedProfiles: profilesFor(plan.Services, services),
	}
	payload, err := apps.CreatePayload(apps.CreateRequest{
		AppName:  name,
		Project:  project,
		Env:      env,
		HeadSHA:  co.HeadSHA,
		Visitors: a.visitors,
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
	return a.printCreated(body, env, public)
}

// profilesFor lists the compose profiles the chosen services need: core
// rejects an enabled service whose profiles are all unselected.
func profilesFor(plan []apps.PlanService, chosen []string) []string {
	var profiles []string
	for _, s := range plan {
		if s.EnabledByDefault || !slices.Contains(chosen, s.Name) {
			continue
		}
		for _, p := range s.Profiles {
			if !slices.Contains(profiles, p) {
				profiles = append(profiles, p)
			}
		}
	}
	return profiles
}

// withoutBypassToken drops every bypass_token from a JSON body before it is
// printed: it gets past the app's login gate.
func withoutBypassToken(body []byte) []byte {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return body
	}
	var drop func(any)
	drop = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			delete(t, "bypass_token")
			for _, child := range t {
				drop(child)
			}
		case []any:
			for _, child := range t {
				drop(child)
			}
		}
	}
	drop(v)
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
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
func (a *adder) chooseRoutes(plan apps.PlanResponse, services []string) (map[string]*string, error) {
	// The plan's routes come from the compose file's labels, named ones
	// included, and apply already: sending them back as shipyard.route would
	// duplicate them. Core offers candidates only when the file has no route.
	var candidates []string
	for _, c := range plan.RouteCandidates {
		if s := string(c); s != "" && slices.Contains(services, s) {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	for {
		answer, err := a.ask(fmt.Sprintf("Service for the %s route (%s)", mainRoute, strings.Join(candidates, ", ")), candidates[0])
		if err != nil {
			return nil, err
		}
		if !slices.Contains(candidates, answer) {
			a.printf("Choose one of %s\n", strings.Join(candidates, ", "))
			continue
		}
		route := mainRoute
		return map[string]*string{answer: &route}, nil
	}
}

// chooseEnv asks, name by name, which env vars to set now and reads their
// values. Values are never echoed or printed.
func (a *adder) chooseEnv(names []apps.EnvName) ([]apps.EnvItem, error) {
	var items []apps.EnvItem
	for _, e := range names {
		// A value set with no services reaches every service, sidecars included
		if len(e.Services) == 0 {
			a.printf("%s is used by no service in the plan, so it is not asked for: set it in the Shipyard dashboard.\n", e.Name)
			continue
		}
		ok, err := a.confirm(fmt.Sprintf("Set %s (%s) for %s now?", e.Name, kindLabel(e.Kind), strings.Join(e.Services, ", ")), false)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		// An empty value would clear the variable; answer n above to skip it
		var value string
		for value == "" {
			a.printf("Value for %s (one line, not shown): ", e.Name)
			value, err = a.readSecret(a.in)
			a.printf("\n")
			if err != nil {
				return nil, fmt.Errorf("reading the value of %q: %w", e.Name, err)
			}
			if value == "" {
				a.printf("A value is required.\n")
			}
		}
		kind := e.Kind
		if kind == "" {
			kind = apps.KindRuntime
		}
		// Stored hidden, so the value is not readable back through the API or dashboard
		hidden := true
		items = append(items, apps.EnvItem{Name: e.Name, Value: value, Kind: kind, Hidden: &hidden, Services: e.Services})
	}
	return items, nil
}

// askAppName asks for the app name until core would accept it, before any env
// value is typed: a rejected name would otherwise cost the user every value.
// replaceable is the plan's existing app, whose name core frees when it
// replaces that app.
func (a *adder) askAppName(def, replaceable string) (string, error) {
	for {
		name, err := a.ask("App name", def)
		if err != nil {
			return "", err
		}
		if !apps.ValidAppName(name) {
			a.printf("App names may use letters, digits, dashes, underscores and spaces (at most 255)\n")
			continue
		}
		if name == replaceable {
			return name, nil
		}
		// A failed lookup is not a reason to stop: core checks again on create
		if taken, err := apps.NameTaken(a.client, name); err == nil && taken {
			a.printf("An app named %q already exists in this org: choose another name\n", name)
			continue
		}
		return name, nil
	}
}

// existingAppName is the name in the plan's existing_app, or "".
func existingAppName(raw json.RawMessage) string {
	var app struct {
		Name string `json:"name"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &app) != nil {
		return ""
	}
	return app.Name
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
	return a.yesNo(prompt, &def)
}

// confirmExplicit asks until the answer is y or n: an empty line is asked again.
func (a *adder) confirmExplicit(prompt string) (bool, error) {
	return a.yesNo(prompt, nil)
}

func (a *adder) yesNo(prompt string, def *bool) (bool, error) {
	hint := "y/n"
	if def != nil && *def {
		hint = "Y/n"
	} else if def != nil {
		hint = "y/N"
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
			if def != nil {
				return *def, nil
			}
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
	body = withoutBypassToken(apps.RedactValues(body, env, public...))
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
			a.printf("%s:\n", key)
			a.printBlock(indentJSON(resp[key]) + "\n")
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
	// Core applies none of this request's values on these answers
	if len(env) > 0 && (action.Status == apps.StatusAlreadyExists || action.Status == apps.StatusCreating) {
		names := make([]string, 0, len(env))
		for _, e := range env {
			names = append(names, e.Name)
		}
		a.printf("Not set: %s. Set them on the app in the Shipyard dashboard.\n", strings.Join(names, ", "))
	}
}

// printCreated shows the created app. A 2xx without the app, such as a proxy
// page, is no evidence of a create, so it is an error.
func (a *adder) printCreated(body []byte, env []apps.EnvItem, public []string) error {
	body = withoutBypassToken(apps.RedactValues(body, env, public...))
	var resp struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Data.ID == "" {
		return fmt.Errorf("shipyard's answer has no app, so the outcome is unknown: check the dashboard before running shipyard add again")
	}
	a.printf("Created app %s (%s). Its first build is starting.\n", resp.Data.Attributes.Name, resp.Data.ID)
	if resp.Data.Attributes.URL != "" {
		a.printf("URL: %s\n", resp.Data.Attributes.URL)
	}
	return nil
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
