package version

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-musicfox/go-musicfox/internal/types"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCheckUpdateUsesUpstreamBase(t *testing.T) {
	oldClient, oldVersion := http.DefaultClient, types.AppVersion
	t.Cleanup(func() { http.DefaultClient, types.AppVersion = oldClient, oldVersion })
	for _, displayVersion := range []string{"v5.0.0", "599e682-dev", "v5.1.0+cassette"} {
		for _, tc := range []struct {
			tag  string
			want bool
		}{{types.UpstreamBaseVersion, false}, {"v9.9.9", true}} {
			t.Run(displayVersion+"/"+tc.tag, func(t *testing.T) {
				types.AppVersion = displayVersion
				http.DefaultClient = &http.Client{Transport: releaseTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != types.AppCheckUpdateUrl {
						t.Fatalf("unexpected release URL: %s", r.URL)
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"tag_name":"` + tc.tag + `"}`))}, nil
				})}
				if update, tag := CheckUpdate(); update != tc.want || tag != tc.tag {
					t.Fatalf("CheckUpdate() = (%v, %q), want (%v, %q)", update, tag, tc.want, tc.tag)
				}
			})
		}
	}
}
