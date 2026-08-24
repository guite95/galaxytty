package remote

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/brutella/dnssd"
)

func TestDiscoveryResolvesIPv4Service(t *testing.T) {
	discovery := &Discovery{
		Timeout: time.Second,
		lookup: func(ctx context.Context, service string, add dnssd.AddFunc, _ dnssd.RmvFunc) error {
			if service != GalaxyService {
				return errors.New("unexpected browse target")
			}
			add(dnssd.BrowseEntry{Port: 43127, IPs: []net.IP{net.ParseIP("192.0.2.15")}})
			<-ctx.Done()
			return ctx.Err()
		},
	}
	address, err := discovery.Resolve(context.Background())
	if err != nil || address != "192.0.2.15:43127" {
		t.Fatalf("address=%q err=%v", address, err)
	}
}

func TestDiscoveryUsesNativeResolverWhenConfigured(t *testing.T) {
	discovery := &Discovery{
		Timeout: time.Second,
		native: func(context.Context) (string, error) {
			return "Galaxy.local.:43127", nil
		},
	}
	address, err := discovery.Resolve(context.Background())
	if err != nil || address != "Galaxy.local.:43127" {
		t.Fatalf("address=%q err=%v", address, err)
	}
}

func TestParseDNSServiceBrowseAndLookupLines(t *testing.T) {
	instance, ok := parseDNSServiceBrowseLine(
		"10:42:06.460  Add  2  12 local.  _galaxytty._tcp.  GalaxyTTY-SM-A376N",
	)
	if !ok || instance != "GalaxyTTY-SM-A376N" {
		t.Fatalf("instance=%q ok=%t", instance, ok)
	}
	address, ok := parseDNSServiceLookupLine(
		"10:42:51.456 GalaxyTTY-SM-A376N._galaxytty._tcp.local. can be reached at Android_H1U4DUH6.local.:44235 (interface 12)",
	)
	if !ok || address != "Android_H1U4DUH6.local.:44235" {
		t.Fatalf("address=%q ok=%t", address, ok)
	}
	for _, line := range []string{
		"10:42:06.459 ...STARTING...",
		"10:42:51.456 service can be reached at host.local.:0",
		"10:42:51.456 service can be reached at host.local.:70000",
	} {
		if _, ok := parseDNSServiceBrowseLine(line); ok {
			t.Fatalf("unexpected browse parse for %q", line)
		}
		if _, ok := parseDNSServiceLookupLine(line); ok {
			t.Fatalf("unexpected lookup parse for %q", line)
		}
	}
}

func TestDiscoveryTimesOutWithoutService(t *testing.T) {
	discovery := &Discovery{
		Timeout: 5 * time.Millisecond,
		lookup: func(ctx context.Context, _ string, _ dnssd.AddFunc, _ dnssd.RmvFunc) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	_, err := discovery.Resolve(context.Background())
	if !errors.Is(err, ErrDiscoveryTimeout) {
		t.Fatalf("err=%v", err)
	}
}

func TestServiceAddressRejectsUnusableAdvertisement(t *testing.T) {
	_, err := serviceAddress(dnssd.BrowseEntry{Port: 0})
	if !errors.Is(err, ErrInvalidService) {
		t.Fatalf("err=%v", err)
	}
}
