package client

import "github.com/shipyard/shipyard-cli/pkg/requests"

type Client struct {
	Requester   requests.Requester
	OrgLookupFn func() string
}

func New(r requests.Requester, orgLookupFn func() string) Client {
	return Client{Requester: r, OrgLookupFn: orgLookupFn}
}
