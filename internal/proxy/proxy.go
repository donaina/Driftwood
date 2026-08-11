package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type Proxy struct {
	target *url.URL
}

// Get the targeted url and return it
func New(target string) (*Proxy, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	return &Proxy{
		target: targetURL,
	}, nil
}

func (p *Proxy) Handler() http.HandlerFunc {
	proxy := httputil.NewSingleHostReverseProxy(p.target)

	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[ApiDiff] %s %s",
			r.Method,
			r.URL.Path,
		)
		proxy.ServeHTTP(w, r)
	}
}
