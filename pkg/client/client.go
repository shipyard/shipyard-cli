package client

type Requester interface {
	Do(method string, uri string, body any) ([]byte, error)
}

type Client struct {
	Requester Requester
	Org       string
}

func New(r Requester, org string) Client {
	return Client{Requester: r, Org: org}
}
