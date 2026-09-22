package netguard

/* The SSRF rules: which destinations this process is willing to open an outbound
   connection to, and on whose say-so.

   A leaf package — it imports nothing from Driftwood — because two different
   callers now need the same rules for different reasons. internal/proxy applies
   them to the backend it forwards to; internal/webhook applies them to the URL an
   operator configures for alert delivery. Those two have an unavoidable
   dependency in one direction (the proxy hands alerts to the deliverer), so the
   guards could not stay in the proxy without the deliverer importing the proxy
   back. Duplicating them was the alternative, and two copies of a security rule
   drift the first time one is tightened.

   Nothing here dials. ParseAndValidate judges a URL string, and DialContext
   judges an address at the moment of connection; the note on the latter says why
   both checks are needed rather than either alone. */

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ErrInvalidTarget reports that a target was refused on its merits: a scheme
// that is not http or https, or a host the SSRF rules will not dial.
//
// It is a sentinel because the caller has to tell this apart from a failure to
// *record* a target that was accepted. Both are errors and neither is a 200, but
// they are different answers to the client — 400 for "you asked for something I
// will not do", 500 for "I agreed and could not write it down" — and a handler
// that cannot distinguish them reports the second as the first. That is not
// hypothetical: this feature's first version surfaced an unwritable data
// directory as "invalid target URL", and the existing test for that route is
// what said so.
var ErrInvalidTarget = errors.New("invalid target")

// ParseAndValidate reads a URL and refuses one this process will not dial.
//
// allowPrivate is the caller's assertion that whoever named this target is the
// operator rather than a request that arrived over the wire — see IsBlockedHost
// for why that distinction, and not public-versus-private, is the one being made.
func ParseAndValidate(raw string, allowPrivate bool) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("target URL cannot be empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("scheme must be http or https, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("URL must have a host")
	}

	if !allowPrivate && IsBlockedHost(parsed.Hostname()) {
		return nil, fmt.Errorf("target host %q is blocked (SSRF protection)", parsed.Hostname())
	}
	return parsed, nil
}

// IsBlockedHost reports whether a target host names infrastructure the proxy
// has no business reaching.
//
// Only the operator-facing setter consults this. The constructor does not,
// because Driftwood's own default target is http://localhost:3000 — it is a
// local dev tool, and a rule that blocked private hosts unconditionally would
// reject its primary use. The line that matters is not private-versus-public but
// who named the target: an operator typing a flag, or a request that arrived
// over the wire.
func IsBlockedHost(host string) bool {
	// Hostnames are case-insensitive, and a trailing dot is a legal fully
	// qualified spelling of the same name. A plain string compare let both
	// spellings of "localhost" through.
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return true
	}

	switch host {
	case "localhost", "localhost.localdomain", "ip6-localhost", "ip6-loopback":
		return true
	case "metadata.google.internal", "metadata.goog":
		// The GCP metadata service answers to these names, which are not IP
		// literals, so nothing below would catch them.
		return true
	}

	if ip := ParseIPLiteral(host); ip != nil {
		return IsBlockedIP(ip)
	}

	// A name with no dot resolves against the search domains, so it names
	// something inside the network Driftwood runs in rather than a host on the
	// public internet. A dot is not proof of safety, but its absence is proof
	// of locality.
	return !strings.Contains(host, ".")
}

// IsBlockedIP reports whether an address is one Driftwood will not dial. The
// stdlib predicates cover the ranges the hand-rolled octet comparisons did,
// plus IPv6 ULA (fd00::/8) and the unspecified address, both of which were
// reachable before.
func IsBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

// ParseIPLiteral reads the spellings a resolver accepts for an address but
// net.ParseIP does not: the single-integer decimal and hexadecimal forms. Both
// 2130706433 and 0x7f000001 mean 127.0.0.1 to getaddrinfo, which is what
// actually dials them, so a check that only understood dotted quads waved them
// straight through.
func ParseIPLiteral(host string) net.IP {
	if ip := net.ParseIP(host); ip != nil {
		return ip
	}

	base, digits := 10, host
	if strings.HasPrefix(host, "0x") || strings.HasPrefix(host, "0X") {
		base, digits = 16, host[2:]
	}
	if digits == "" {
		return nil
	}
	n, err := strconv.ParseUint(digits, base, 32)
	if err != nil {
		return nil
	}
	return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

// lookupIP resolves a host to its addresses.
//
// A var rather than a direct call only so a test can decide what a name
// resolves to. The rebinding case this guards against is a property of *two
// different answers* to the same question, which cannot be staged against a real
// resolver without controlling DNS. Nothing outside a test writes it.
var lookupIP = net.DefaultResolver.LookupIPAddr

// DialContext wraps a dialer so that a connection is refused when the address it
// actually resolves to is one IsBlockedIP rejects.
//
// ParseAndValidate judges the URL the operator typed; this judges the address
// the resolver returned, and the two are not the same thing. A hostname that
// passes the string check can still resolve to 127.0.0.1 — a name the attacker
// controls, pointed at loopback, or a public name whose authoritative server
// answers differently on the second lookup (DNS rebinding; the TTL is set to 0
// so the check and the dial get different answers). Checking the URL and then
// dialling the name leaves that window open for the whole connection.
//
// It is deliberately a wrapper rather than a replacement: callers keep their own
// timeouts and keep-alive settings and hand the base dialer in, so this adds a
// check without becoming a second place where dial policy is decided.
func DialContext(base *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if base == nil {
		base = &net.Dialer{}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		// Resolve, check, and then dial the checked address itself — never the
		// name. Handing the name to the base dialer would resolve it a second
		// time, and the second answer is exactly what an attacker controls: the
		// check would pass on the first reply and the connection would be made
		// to whatever the second one said. Connecting to the address we vetted
		// is what makes the check mean anything.
		//
		// TLS is unaffected: http.Transport sets tls.Config.ServerName from the
		// URL's host, so certificate verification still names the host the
		// operator typed even though the socket went to a literal address.
		ips, err := lookupIP(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if IsBlockedIP(ip.IP) {
				return nil, fmt.Errorf("host %q resolves to blocked address %s (SSRF protection)", host, ip.IP)
			}
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("host %q did not resolve to any address", host)
		}

		return base.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
}
