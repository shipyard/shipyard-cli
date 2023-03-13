package requests

import "testing"

func TestParseErrorResponse(t *testing.T) {
	tests := []struct {
		resp []byte
		want string
	}{
		{
			resp: nil,
			want: "",
		},
		{
			resp: []byte{},
			want: "",
		},
		{
			resp: []byte(
				`{
				    "attributes": {
				     	"name": "foo"
				    },
				    "id": "1234",
				    "type": "org"
    			}`,
			),
			want: "",
		},
		{
			resp: []byte(
				`{
					"errors": [
					    {
					     	"status": 404,
					     	"title": "Environment not found"
					    }
					]
				}`,
			),
			want: "Environment not found",
		},
		{
			resp: []byte(
				`{
					"errors": [
					    {
					     	"status": 404,
					        "title": "User org not found"
					    }
					]
				}`,
			),
			want: "User org not found",
		},
	}

	for _, test := range tests {
		got := parseError(test.resp)
		if got != test.want {
			t.Errorf("want %s, but got %s", test.want, got)
		}
	}
}
