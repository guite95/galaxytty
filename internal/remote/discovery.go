package remote

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/brutella/dnssd"
)

const GalaxyService = "_galaxytty._tcp.local."

var (
	ErrDiscoveryTimeout = errors.New("Galaxy Helper discovery timed out")
	ErrInvalidService   = errors.New("Galaxy Helper advertised an invalid endpoint")
)

type lookupFunc func(context.Context, string, dnssd.AddFunc, dnssd.RmvFunc) error
type nativeDiscoveryFunc func(context.Context) (string, error)

type Discovery struct {
	Timeout time.Duration
	lookup  lookupFunc
	native  nativeDiscoveryFunc
}

func NewDiscovery(timeout time.Duration) *Discovery {
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	discovery := &Discovery{Timeout: timeout, lookup: dnssd.LookupType}
	if runtime.GOOS == "darwin" {
		discovery.native = resolveDarwinDNSService
	}
	return discovery
}

func (discovery *Discovery) Resolve(ctx context.Context) (string, error) {
	if discovery == nil {
		return "", errors.New("Galaxy Helper discovery is nil")
	}
	timeout := discovery.Timeout
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	resolveContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if discovery.native != nil {
		address, err := discovery.native(resolveContext)
		if err == nil {
			return address, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if resolveContext.Err() != nil {
			return "", ErrDiscoveryTimeout
		}
		return "", fmt.Errorf("discover Galaxy Helper with macOS DNS-SD: %w", err)
	}
	lookup := discovery.lookup
	if lookup == nil {
		lookup = dnssd.LookupType
	}
	entries := make(chan dnssd.BrowseEntry, 8)
	errorsChannel := make(chan error, 1)
	go func() {
		errorsChannel <- lookup(
			resolveContext,
			GalaxyService,
			func(entry dnssd.BrowseEntry) {
				select {
				case entries <- entry:
				case <-resolveContext.Done():
				}
			},
			func(dnssd.BrowseEntry) {},
		)
	}()

	for {
		select {
		case <-resolveContext.Done():
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", ErrDiscoveryTimeout
		case err := <-errorsChannel:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				return "", fmt.Errorf("browse for Galaxy Helper: %w", err)
			}
		case entry := <-entries:
			address, err := serviceAddress(entry)
			if err == nil {
				return address, nil
			}
		}
	}
}

func resolveDarwinDNSService(ctx context.Context) (string, error) {
	instance, err := readDNSService(ctx, []string{"-B", "_galaxytty._tcp", "local."}, parseDNSServiceBrowseLine)
	if err != nil {
		return "", err
	}
	return readDNSService(
		ctx,
		[]string{"-L", instance, "_galaxytty._tcp", "local."},
		parseDNSServiceLookupLine,
	)
}

func readDNSService(
	ctx context.Context,
	arguments []string,
	parse func(string) (string, bool),
) (string, error) {
	command := exec.CommandContext(ctx, "dns-sd", arguments...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("read dns-sd output: %w", err)
	}
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("start dns-sd: %w", err)
	}
	defer func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if value, ok := parse(scanner.Text()); ok {
			return value, nil
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return "", fmt.Errorf("scan dns-sd output: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "", errors.New("dns-sd stopped before resolving Galaxy Helper")
}

func parseDNSServiceBrowseLine(line string) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 7 || fields[1] != "Add" || fields[4] != "local." || fields[5] != "_galaxytty._tcp." {
		return "", false
	}
	instance := strings.TrimSpace(strings.Join(fields[6:], " "))
	return instance, instance != ""
}

func parseDNSServiceLookupLine(line string) (string, bool) {
	const marker = " can be reached at "
	markerIndex := strings.Index(line, marker)
	if markerIndex < 0 {
		return "", false
	}
	fields := strings.Fields(line[markerIndex+len(marker):])
	if len(fields) == 0 {
		return "", false
	}
	host, portText, err := net.SplitHostPort(fields[0])
	if err != nil || strings.TrimSpace(host) == "" {
		return "", false
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", false
	}
	return net.JoinHostPort(host, portText), true
}

func serviceAddress(entry dnssd.BrowseEntry) (string, error) {
	if entry.Port <= 0 || entry.Port > 65535 {
		return "", ErrInvalidService
	}
	for _, address := range entry.IPs {
		if address != nil && address.To4() != nil && !address.IsUnspecified() {
			return net.JoinHostPort(address.String(), fmt.Sprint(entry.Port)), nil
		}
	}
	for _, address := range entry.IPs {
		if address != nil && !address.IsUnspecified() && !address.IsLinkLocalUnicast() {
			return net.JoinHostPort(address.String(), fmt.Sprint(entry.Port)), nil
		}
	}
	if entry.Host != "" {
		return net.JoinHostPort(entry.Host, fmt.Sprint(entry.Port)), nil
	}
	return "", ErrInvalidService
}
