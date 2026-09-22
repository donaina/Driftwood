package netguard

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

/* The authoritative table for the SSRF rules.

   These rules were previously exercised only indirectly, through
   proxy.SetProjectTarget and TestProxySSRFValidation. That test is valuable and
   still runs — it is the proof the move to this package changed no behaviour —
   but it asserts on a method that has other reasons to fail, and it can only
   cover the destinations someone thought to write into a proxy test. This table
   is the rules themselves. */

func TestIsBlockedHost(t *testing.T) {
	blocked := []string{
		// Loopback by name, in the spellings a resolver accepts for it.
		"localhost", "LOCALHOST", "localhost.", "localhost.localdomain",
		"ip6-localhost", "ip6-loopback",
		// Loopback by address. Note these are bare hostnames, not bracketed
		// ones: callers pass url.URL.Hostname(), which has already stripped the
		// brackets from a v6 literal.
		"127.0.0.1", "127.0.0.2", "::1",
		// The spellings net.ParseIP does not read but getaddrinfo does.
		"2130706433", "0x7f000001", "0X7F000001",
		// Link-local, including the cloud metadata endpoint.
		"169.254.169.254",
		// RFC1918.
		"10.0.0.1", "172.16.0.1", "192.168.1.1",
		// IPv6 ULA and unspecified.
		"fd00::1", "0.0.0.0", "::",
		// Metadata services that answer to names rather than addresses.
		"metadata.google.internal", "metadata.goog",
		// No dot: resolves against the search domains, so it names something
		// inside the network Driftwood runs in.
		"intranet", "db",
		// Nothing at all.
		"",
	}

	for _, host := range blocked {
		if !IsBlockedHost(host) {
			t.Errorf("IsBlockedHost(%q) = false, want true", host)
		}
	}

	allowed := []string{
		"hooks.slack.com",
		"discord.com",
		"api.github.com",
		"example.com",
		"93.184.216.34",
		"2606:2800:220:1:248:1893:25c8:1946", // a public IPv6 address
		"outbound.example.co.uk",
	}

	for _, host := range allowed {
		if IsBlockedHost(host) {
			t.Errorf("IsBlockedHost(%q) = true, want false", host)
		}
	}
}

func TestParseAndValidate(t *testing.T) {
	t.Run("refuses", func(t *testing.T) {
		bad := []struct {
			raw    string
			reason string
		}{
			{"", "empty"},
			{"   ", "whitespace only"},
			{"not-a-url", "no scheme"},
			{"file:///etc/passwd", "file scheme"},
			{"ftp://example.com", "ftp scheme"},
			{"gopher://example.com", "gopher scheme"},
			{"javascript:alert(1)", "javascript scheme"},
			{"data:text/html,<script>alert(1)</script>", "data scheme"},
			{"http://", "no host"},
			{"http://localhost:8787", "loopback"},
			{"http://169.254.169.254/latest/meta-data/", "link-local metadata"},
			{"http://10.0.0.1/", "private"},
			{"http://2130706433/", "decimal loopback"},
			{"http://0x7f000001/", "hex loopback"},
			{"http://metadata.google.internal/", "GCP metadata by name"},
			{"http://intranet/", "dotless"},
		}
		for _, c := range bad {
			if _, err := ParseAndValidate(c.raw, false); err == nil {
				t.Errorf("ParseAndValidate(%q, false) = nil error, want refusal (%s)", c.raw, c.reason)
			}
		}
	})

	t.Run("allows", func(t *testing.T) {
		good := []string{
			"http://example.com",
			"https://api.example.com",
			"https://hooks.slack.com/services/T00/B00/XXXX",
			"https://discord.com/api/webhooks/1234/token-shaped-segment",
			"http://example.com:8080/path?query=1",
		}
		for _, raw := range good {
			if _, err := ParseAndValidate(raw, false); err != nil {
				t.Errorf("ParseAndValidate(%q, false) = %v, want nil", raw, err)
			}
		}
	})

	/* allowPrivate is the operator-named-versus-wire distinction, and it is the
	   whole reason this parameter exists: Driftwood's own default target is
	   localhost, so a rule that refused private hosts unconditionally would
	   reject the product's primary use. A test that only ever passed false would
	   let the parameter be inverted without noticing. */
	t.Run("allows private when the operator named it", func(t *testing.T) {
		private := []string{
			"http://localhost:3000",
			"http://127.0.0.1:8080",
			"http://192.168.1.10:9000",
			"http://10.0.0.5",
		}
		for _, raw := range private {
			if _, err := ParseAndValidate(raw, true); err != nil {
				t.Errorf("ParseAndValidate(%q, true) = %v, want nil", raw, err)
			}
		}
	})

	/* allowPrivate is not a blanket amnesty. The scheme and host checks are
	   unconditional: a private-target allowance must not turn
	   file:///etc/passwd into a valid target. */
	t.Run("allowPrivate does not excuse a bad scheme", func(t *testing.T) {
		for _, raw := range []string{"file:///etc/passwd", "ftp://localhost/", "not-a-url"} {
			if _, err := ParseAndValidate(raw, true); err == nil {
				t.Errorf("ParseAndValidate(%q, true) = nil error, want refusal", raw)
			}
		}
	})
}

func TestParseIPLiteral(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1":  "127.0.0.1",
		"2130706433": "127.0.0.1",
		"0x7f000001": "127.0.0.1",
		"0X7F000001": "127.0.0.1",
		"::1":        "::1",
	}
	for in, want := range cases {
		ip := ParseIPLiteral(in)
		if ip == nil {
			t.Errorf("ParseIPLiteral(%q) = nil, want %s", in, want)
			continue
		}
		if ip.String() != want {
			t.Errorf("ParseIPLiteral(%q) = %s, want %s", in, ip, want)
		}
	}

	for _, in := range []string{"", "hooks.slack.com", "0x", "not-an-ip"} {
		if ip := ParseIPLiteral(in); ip != nil {
			t.Errorf("ParseIPLiteral(%q) = %s, want nil", in, ip)
		}
	}
}

// DialContext is what closes the window ParseAndValidate cannot see: a name that
// passes the string check and resolves to loopback. The test controls the
// resolver because that is the only way to stage it — a real name either points
// where it points or needs DNS this test does not own.
func TestDialContextRefusesResolvedLoopback(t *testing.T) {
	cases := []struct {
		name string
		addr string
		why  string
	}{
		{"loopback", "127.0.0.1", "the classic rebinding target"},
		{"ipv6 loopback", "::1", "the same, over v6"},
		{"link-local metadata", "169.254.169.254", "cloud metadata"},
		{"private", "10.1.2.3", "the LAN behind Driftwood"},
		{"ULA", "fd00::1", "IPv6 private"},
		{"unspecified", "0.0.0.0", "dials loopback on Linux"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			restore := stubResolver(t, func(_ context.Context, _ string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP(c.addr)}}, nil
			})
			defer restore()

			dial := DialContext(&net.Dialer{})
			conn, err := dial(context.Background(), "tcp", "rebind.example.com:80")
			if err == nil {
				conn.Close()
				t.Fatalf("dial to a name resolving to %s was allowed (%s)", c.addr, c.why)
			}
			if !strings.Contains(err.Error(), "SSRF protection") {
				t.Errorf("refusal did not say why: %v", err)
			}
		})
	}
}

// The property that makes the check mean anything: the connection must go to the
// address that was vetted, not to the name it came from. Handing the name back
// to the base dialer would resolve it a second time, and an attacker who
// controls the answer controls that second one.
//
// The base dialer is given a port nothing listens on, so the dial fails — and
// the error it returns names the address it tried. That name is the assertion.
func TestDialContextDialsTheVettedAddress(t *testing.T) {
	const publicIP = "93.184.216.34"

	restore := stubResolver(t, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP(publicIP)}}, nil
	})
	defer restore()

	dial := DialContext(&net.Dialer{Timeout: 200 * time.Millisecond})
	conn, err := dial(context.Background(), "tcp", "harmless.example.com:1")
	if err == nil {
		conn.Close()
		t.Fatal("something was listening on a public address on port 1")
	}

	if !strings.Contains(err.Error(), publicIP) {
		t.Errorf("the dial did not name the resolved address %s — it may have been handed the hostname instead: %v", publicIP, err)
	}
	if strings.Contains(err.Error(), "harmless.example.com") {
		t.Errorf("the dial went to the hostname rather than the vetted address: %v", err)
	}
}

// A name that resolves to nothing is a failure, not a pass. An empty address
// list would otherwise fall through to "no blocked addresses found".
func TestDialContextRefusesEmptyResolution(t *testing.T) {
	restore := stubResolver(t, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return nil, nil
	})
	defer restore()

	if conn, err := DialContext(&net.Dialer{})(context.Background(), "tcp", "nowhere.example.com:80"); err == nil {
		conn.Close()
		t.Fatal("dial to a name that resolved to nothing was allowed")
	}
}

// A resolver failure must surface rather than being read as "no blocked
// addresses".
func TestDialContextPropagatesResolverFailure(t *testing.T) {
	restore := stubResolver(t, func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return nil, errors.New("no such host")
	})
	defer restore()

	_, err := DialContext(&net.Dialer{})(context.Background(), "tcp", "nx.example.com:80")
	if err == nil || !strings.Contains(err.Error(), "no such host") {
		t.Fatalf("resolver error was not propagated: %v", err)
	}
}

// stubResolver replaces the package's resolver seam for one test and returns a
// function that puts the real one back.
func stubResolver(t *testing.T, fn func(context.Context, string) ([]net.IPAddr, error)) func() {
	t.Helper()
	prev := lookupIP
	lookupIP = fn
	return func() { lookupIP = prev }
}
