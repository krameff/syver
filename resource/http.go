package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/krameff/syver/system"
	"github.com/krameff/syver/util"
	"github.com/urfave/cli/v3"
)

type HTTP struct {
	DiscoveryMeta      `yaml:",inline" json:",inline"`
	Title              string              `json:"title,omitempty" yaml:"title,omitempty"`
	Meta               meta                `json:"meta,omitempty" yaml:"meta,omitempty"`
	id                 string              `json:"-" yaml:"-"`
	URL                string              `json:"url,omitempty" yaml:"url,omitempty"`
	Method             string              `json:"method,omitempty" yaml:"method,omitempty"`
	Status             matcher             `json:"status" yaml:"status"`
	AllowInsecure      bool                `json:"allow-insecure" yaml:"allow-insecure"`
	NoFollowRedirects  bool                `json:"no-follow-redirects" yaml:"no-follow-redirects"`
	Timeout            int                 `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	RequestHeader      []string            `json:"request-headers,omitempty" yaml:"request-headers,omitempty"`
	RequestQueryParams map[string][]string `json:"request-query-params,omitempty" yaml:"request-query-params,omitempty"`
	RequestBody        string              `json:"request-body,omitempty" yaml:"request-body,omitempty"`
	Headers            matcher             `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body               matcher             `json:"body,omitempty" yaml:"body,omitempty"`
	Username           string              `json:"username,omitempty" yaml:"username,omitempty"`
	Password           string              `json:"password,omitempty" yaml:"password,omitempty"`
	CAFile             string              `json:"ca-file,omitempty" yaml:"ca-file,omitempty"`
	CertFile           string              `json:"cert-file,omitempty" yaml:"cert-file,omitempty"`
	KeyFile            string              `json:"key-file,omitempty" yaml:"key-file,omitempty"`
	Skip               bool                `json:"skip,omitempty" yaml:"skip,omitempty"`
	Proxy              string              `json:"proxy,omitempty" yaml:"proxy,omitempty"`
}

const (
	HTTPResourceKey  = "http"
	HTTPResourceName = "HTTP"
)

func init() {
	Register(Descriptor{
		Key:          HTTPResourceKey,
		Name:         HTTPResourceName,
		New:          func() Resource { return &HTTP{} },
		InValidation: true,
		InDiscovery:  true,
		AppendSys: func(sys *system.System, key string, config util.Config) (Resource, error) {
			r := &HTTP{}
			if _, err := r.fromSystem(sys, key, config); err != nil {
				return nil, err
			}
			return r, nil
		},
		CLIFlags: func() []cli.Flag {
			return []cli.Flag{
				&cli.BoolFlag{Name: "insecure", Aliases: []string{"k"}},
				&cli.BoolFlag{Name: "no-follow-redirects", Aliases: []string{"r"}},
				&cli.DurationFlag{Name: "timeout", Value: 5 * time.Second},
				&cli.StringFlag{Name: "username", Aliases: []string{"u"}, Usage: "Username for basic auth"},
				&cli.StringFlag{Name: "password", Aliases: []string{"p"}, Usage: "Password for basic auth"},
				&cli.StringFlag{Name: "proxy", Aliases: []string{"x"}, Usage: "Proxy server to use. e.g. http://10.0.0.2:8080"},
			}
		},
	})
}

func (h *HTTP) ID() string {
	if h.URL != "" && h.URL != h.id {
		return fmt.Sprintf("%s: %s", h.id, h.URL)
	}
	return h.id
}
func (h *HTTP) SetID(id string)  { h.id = id }
func (u *HTTP) SetSkip()         { u.Skip = true }
func (u *HTTP) TypeKey() string  { return HTTPResourceKey }
func (u *HTTP) TypeName() string { return HTTPResourceName }

// FIXME: Can this be refactored?
func (r *HTTP) GetTitle() string { return r.Title }
func (r *HTTP) GetMeta() meta    { return r.Meta }
func (r *HTTP) getURL() string {
	if r.URL != "" {
		return r.URL
	}
	return r.id
}

func (u *HTTP) Validate(ctx context.Context, sys *system.System) []TestResult {
	ctx = context.WithValue(ctx, idKey{}, u.ID())
	skip := u.Skip
	if u.Timeout == 0 {
		u.Timeout = 5000
	}
	sysHTTP := sys.NewHTTP(ctx, u.getURL(), sys, util.Config{
		AllowInsecure:     u.AllowInsecure,
		CAFile:            u.CAFile,
		CertFile:          u.CertFile,
		KeyFile:           u.KeyFile,
		NoFollowRedirects: u.NoFollowRedirects,
		Timeout:           time.Duration(u.Timeout) * time.Millisecond, Username: u.Username, Password: u.Password, Proxy: u.Proxy,
		RequestHeader: u.RequestHeader, RequestBody: u.RequestBody, RequestQueryParams: u.RequestQueryParams, Method: u.Method})
	sysHTTP.SetAllowInsecure(u.AllowInsecure)
	sysHTTP.SetNoFollowRedirects(u.NoFollowRedirects)
	defer sysHTTP.Close()

	var results []TestResult
	results = append(results, ValidateValue(u, "status", u.Status, sysHTTP.Status, skip))
	if shouldSkip(results) {
		skip = true
	}
	if isSetWarnEmpty(u.Headers, fmt.Sprintf("%s: http.headers", u.ID()), skip) {
		results = append(results, ValidateValue(u, "Headers", u.Headers, sysHTTP.Headers, skip))
	}
	if isSetWarnEmpty(u.Body, fmt.Sprintf("%s: http.body", u.ID()), skip) {
		results = append(results, ValidateValue(u, "Body", u.Body, sysHTTP.Body, skip))
	}

	return results
}

func NewHTTP(sysHTTP system.HTTP, config util.Config) (*HTTP, error) {
	http := sysHTTP.HTTP()
	status, err := sysHTTP.Status()
	u := &HTTP{
		id:                 http,
		Status:             status,
		RequestHeader:      []string{},
		RequestQueryParams: nil,
		Headers:            nil,
		// Body is deliberately left unset. `[]` asserts nothing (isSet skips an
		// empty list), so emitting it only writes a line the reader has to think
		// about and dismiss -- and isSetWarnEmpty would warn about a file
		// `syver add` had just written. RequestHeader above is a request *input*,
		// not a matcher, so it is left as-is.
		Body:              nil,
		AllowInsecure:     config.AllowInsecure,
		NoFollowRedirects: config.NoFollowRedirects,
		Timeout:           config.TimeOutMilliSeconds(),
		Username:          config.Username,
		Password:          config.Password,
		Proxy:             config.Proxy,
	}
	return u, err
}

// fromSystem builds a fresh HTTP from live system state, populating the
// receiver in place. It is the one piece of AppendSysResource/
// AppendSysResourceIfExists that genny used to text-substitute per type and
// that ResourceMap's shared generic implementation (resource_map.go) cannot
// derive on its own -- Go generics have no way to pick "NewHTTP" off
// *system.System from a type parameter alone.
func (h *HTTP) fromSystem(sys *system.System, key string, config util.Config) (system.HTTP, error) {
	ctx := context.WithValue(context.Background(), idKey{}, key)
	sysRes := sys.NewHTTP(ctx, key, sys, config)
	n, err := NewHTTP(sysRes, config)
	if err != nil {
		return sysRes, err
	}
	*h = *n
	return sysRes, nil
}
