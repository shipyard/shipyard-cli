package types

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNextPage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "empty url",
			input: "",
			want:  0,
		},
		{
			name:  "page query parameter missing",
			input: "/api/v1/environment?page_size=20",
			want:  0,
		},
		{
			name:  "bad value for page",
			input: "/api/v1/environment?page=2i",
			want:  0,
		},
		{
			name:  "valid url",
			input: "/api/v1/environment?page=2&page_size=5",
			want:  2,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			r := RespManyEnvs{
				Links: Links{
					Next: test.input,
				},
			}
			if got := r.NextPage(); got != test.want {
				t.Errorf(cmp.Diff(got, test.want))
			}
		})
	}
}

func TestParseErrorResponse(t *testing.T) {
	t.Parallel()

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
		got := ParseErrorResponse(test.resp)
		if got != test.want {
			t.Errorf("want %s, but got %s", test.want, got)
		}
	}
}
