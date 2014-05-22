package main 

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	gourl "net/url"
	"os"
	"regexp"
	"strings"

	"github.com/liujianping/bomb/bomber"
	"github.com/liujianping/bomb/bullet"
	"github.com/liujianping/bomb/filter"
)

var (
	flagMethod    = flag.String("m", "GET", "")
	flagHeaders   = flag.String("h", "", "")
	flagReplaces  = flag.String("r", "", "")
	flagD         = flag.String("d", "", "")
	flagType      = flag.String("T", "text/html", "")
	flagAuth      = flag.String("a", "", "")
	flagInsecure  = flag.Bool("allow-insecure", false, "")
	flagLiteral   = flag.Bool("allow-literal", false, "")
	flagOutput    = flag.String("o", "", "")
	flagProxyAddr = flag.String("x", "", "")

	flagC = flag.Int("c", 50, "")
	flagN = flag.Int("n", 200, "")
	flagQ = flag.Int("q", 0, "")
	flagT = flag.Int("t", 0, "")
)

var usage = `Usage: http-bomber [options...] <url>

Options:
  -n  Number of requests to run.
  -c  Number of requests to run concurrently. Total number of requests cannot
      be smaller than the concurency level.
  -q  Rate limit, in seconds (QPS).
  -o  Output type. If none provided, a summary is printed.
      "csv" is the only supported alternative. Dumps the response
      metrics in comma-seperated values format.

  -p  Protocl support, one of HTTP, SMTP, CMD: shell    

  -m  HTTP method, one of GET, POST, PUT, DELETE, HEAD, OPTIONS.
  -h  Custom HTTP headers, name1:value1;name2:value2.
  -d  HTTP request body.
  -T  Content-type, defaults to "text/html".
  -a  Basic authentication, username:password.
  -x  HTTP Proxy address as host:port
  -r  Replace regexp with the dest file line values. -r=regexp1:file1;regexp2:file2

  -regexp-literal Allow regexp only literal replacement.
  -allow-insecure Allow bad/expired TLS/SSL certificates.
`

// Default DNS resolver.
var defaultDnsResolver dnsResolver = &netDnsResolver{}

// DNS resolver interface.
type dnsResolver interface {
	Lookup(domain string) (addr []string, err error)
}

// A DNS resolver based on net.LookupHost.
type netDnsResolver struct{}

// Looks up for the resolved IP addresses of
// the provided domain.
func (*netDnsResolver) Lookup(domain string) (addr []string, err error) {
	return net.LookupHost(domain)
}

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}

	flag.Parse()
	if flag.NArg() < 1 {
		usageAndExit("")
	}

	n := *flagN
	c := *flagC
	q := *flagQ
	t := *flagT

	if n <= 0 || c <= 0 {
		usageAndExit("n and c cannot be smaller than 1.")
	}

	var (
		url, method, originalHost string
		// Username and password for basic auth
		username, password string
		// request headers
		header http.Header = make(http.Header)
		// replacement
		replacement *filter.Replacement
	)

	method = strings.ToUpper(*flagMethod)
	url, originalHost = resolveUrl(flag.Args()[0])

	// set content-type
	header.Set("Content-Type", *flagType)
	// set any other additional headers
	if *flagHeaders != "" {
		headers := strings.Split(*flagHeaders, ";")
		for _, h := range headers {
			re := regexp.MustCompile("([\\w|-]+):(.+)")
			matches := re.FindAllStringSubmatch(h, -1)
			if len(matches) < 1 {
				usageAndExit("")
			}
			header.Set(matches[0][1], matches[0][2])
		}
	}

	// set replacement if set
	if *flagReplaces != "" {
		replacement = filter.NewReplacement(n, *flagLiteral)

		repls := strings.Split(*flagReplaces, ";")
		for _, r := range repls {
			re := regexp.MustCompile("(.+):(.+)")	
			matches := re.FindAllStringSubmatch(r, -1)
			if len(matches) < 1 {
				usageAndExit("")
			}
			exp, err := regexp.Compile(matches[0][1])
			if err != nil {
				usageAndExit(matches[0][1] + " regexp compile: " + err.Error())	
			}

			if err := replacement.Source(exp, matches[0][2]); err != nil {
				usageAndExit(matches[0][2] + " regexp source: " + err.Error())		
			}
		}
	}

	// set basic auth if set
	if *flagAuth != "" {
		re := regexp.MustCompile("(\\w+):(\\w+)")
		matches := re.FindAllStringSubmatch(*flagAuth, -1)
		if len(matches) < 1 {
			usageAndExit("")
		}
		username = matches[0][1]
		password = matches[0][2]
	}

	if *flagOutput != "csv" && *flagOutput != "" {
		usageAndExit("Invalid output type.")
	}

	provider := &bullet.HttpProvider{
		Method: method,
		Url: url,
		Header: header,
		Body: *flagD,
		Username: username,
		Password: password,
		OriginalHost: originalHost,
		AllowInsecure: *flagInsecure,
		ProxyAddr: *flagProxyAddr,
		Replacement: replacement,
	}

	b := &bomber.Bomber{
		N: n,
		C: c,
		Timeout: t,
		Qps: q,
		Output: *flagOutput,
	}

	b.Provider(provider)
	if err := b.Run(); err != nil {
		usageAndExit(err.Error())	
	}
}

// Replaces host with an IP and returns the provided
// string URL as a *url.URL.
//
// DNS lookups are not cached in the package level in Go,
// and it's a huge overhead to resolve a host
// before each request in our case. Instead we resolve
// the domain and replace it with the resolved IP to avoid
// lookups during request time. Supported url strings:
//
// <schema>://google.com[:port]
// <schema>://173.194.116.73[:port]
// <schema>://\[2a00:1450:400a:806::1007\][:port]
func resolveUrl(url string) (string, string) {
	uri, err := gourl.ParseRequestURI(url)
	if err != nil {
		usageAndExit(err.Error())
	}
	originalHost := uri.Host

	serverName, port, err := net.SplitHostPort(uri.Host)
	if err != nil {
		serverName = uri.Host
	}

	addrs, err := defaultDnsResolver.Lookup(serverName)
	if err != nil {
		usageAndExit(err.Error())
	}
	ip := addrs[0]
	if port != "" {
		// join automatically puts square brackets around the
		// ipv6 IPs.
		uri.Host = net.JoinHostPort(ip, port)
	} else {
		uri.Host = ip
		// square brackets are required for ipv6 IPs.
		// otherwise, net.Dial fails with a parsing error.
		if strings.Contains(ip, ":") {
			uri.Host = fmt.Sprintf("[%s]", ip)
		}
	}
	return uri.String(), originalHost
}

func usageAndExit(message string) {
	if message != "" {
		fmt.Fprintf(os.Stderr, message)
		fmt.Fprintf(os.Stderr, "\n\n")
	}
	flag.Usage()
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(1)
}