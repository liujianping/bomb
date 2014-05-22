package bullet

import (
	"errors"
	"net"
	"net/http"
	"io"
	"io/ioutil"
	"crypto/tls"
	"strings"
	"fmt"

	"github.com/liujianping/bomb/bomber"
	"github.com/liujianping/bomb/filter"
)

type HttpProvider struct{
	Method   string
	Url      string
	Header   http.Header
	Body     string
	Username string
	Password string
	// Request host is an resolved IP. TLS/SSL handshakes may require
	// the original server name, keep it to initate the TLS client.
	OriginalHost string
	// Option to allow insecure TLS/SSL certificates.
	AllowInsecure bool
	// Optional address of HTTP proxy server as host:port
	ProxyAddr string
	
	// Replacement
	Replacement *filter.Replacement
}

func (p *HttpProvider) Bullet() bomber.IBullet {

	method := p.Method
	url := p.Url
	body := p.Body

	p.Replacement.Do(&method, &url, &body)
	
	fmt.Println(method, url, body)
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header = p.Header

	// update the Host value in the Request - this is used as the host header in any subsequent request
	req.Host = p.OriginalHost

	if p.Username != "" && p.Password != "" {
		req.SetBasicAuth(p.Username, p.Password)
	}
	return &HttpBullet{req}
}

type HttpBullet struct{
	request *http.Request
}

func (b *HttpBullet) Do(ctx interface{}) *bomber.Result {
	provider, ok := ctx.(*HttpProvider)
	if !ok {
		return bomber.BuildResult(errors.New("HttpBullet Ctx convert type failed."),-1,-1,-1)
	}

	host, _, _ := net.SplitHostPort(provider.OriginalHost)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: provider.AllowInsecure, ServerName: host},
	}
	if provider.ProxyAddr != "" {
		tr.Dial = func(network string, addr string) (conn net.Conn, err error) {
			return net.Dial(network, provider.ProxyAddr)
		}
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Do(b.request)
	step := 0
	code := 0
	var size int64 = -1
	if resp != nil {
		step = 1
		code = resp.StatusCode
		if resp.ContentLength > 0 {
			size = resp.ContentLength
		}
		// consume the whole body
		io.Copy(ioutil.Discard, resp.Body)
		// cleanup body, so the socket can be reusable
		resp.Body.Close()
	}
	
	return bomber.BuildResult(err, step, code, size)	
}

