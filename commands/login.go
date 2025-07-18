package commands

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	"github.com/shipyard/shipyard-cli/auth"
	"github.com/shipyard/shipyard-cli/pkg/display"
)

func NewLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "login",
		Short:        "Log in to the CLI",
		Long:         "This command opens a web browser, prompts you to log in to Shipyard, and saves a new API token in your local config file",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return login()
		},
	}
}

func login() error {
	if _, err := auth.APIToken(); err == nil {
		display.Println("You are already logged in.")
		return nil
	}

	tokenChan := make(chan string)
	errChan := make(chan error)
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := r.URL.Query().Get("token")
		if t == "" {
			errChan <- fmt.Errorf("no token received from Shipyard")
			return
		}
		tokenChan <- t
		fmt.Fprintln(w, "Authentication succeeded. You may close this browser tab.")
	})
	mux.Handle("/", handler)

	//nolint:gosec //local only
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("error creating a local callback server: %w", err)
	}
	listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{
		Addr:              net.JoinHostPort("localhost", strconv.Itoa(port)),
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}

	go func() {
		log.Printf("Trying to start a local callback server on %s.", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			display.Fail(err)
			errChan <- err
			return
		}
		log.Println("Shutting down the local server.")
	}()

	display.Println("Opening the default web browser...")
	backendURL := fmt.Sprintf("https://shipyard.build/api/me/user-token/cli?callbackUrl=http://%s", server.Addr)
	if err := browser.OpenURL(backendURL); err != nil {
		return err
	}
	select {
	case <-time.After(time.Minute):
		return fmt.Errorf("authentication timeout")
	case err := <-errChan:
		return fmt.Errorf("login error: %w", err)
	case t := <-tokenChan:
		if err := SetToken(t, ""); err != nil {
			return err
		}
		display.Println("Login succeeded!")
	}
	return nil
}
