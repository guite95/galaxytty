package doctor

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/provider"
	"github.com/galaxytty/galaxytty/internal/scrcpy"
)

type ADB interface {
	adb.Backend
	Version(context.Context) (string, error)
	MDNSServices(context.Context) ([]adb.MDNSService, error)
}

type Service struct {
	ADB           ADB
	InspectScrcpy func(context.Context, string) (scrcpy.VersionInfo, error)
}

func Run(ctx context.Context, cfg config.Config, selector string) Report {
	client, err := adb.NewClient("", 5*time.Second)
	if err != nil {
		return Report{
			Checks:  []Check{{Name: "adb", Detail: "not found", State: Fail}},
			Summary: "Read mode not ready. Install Android platform-tools and connect an authorized Galaxy.",
		}
	}
	return (Service{ADB: client, InspectScrcpy: scrcpy.Inspect}).Run(ctx, cfg, selector)
}

func (s Service) Run(ctx context.Context, cfg config.Config, selector string) Report {
	report := Report{}
	if s.ADB == nil {
		report.Checks = append(report.Checks, Check{Name: "adb", Detail: "not available", State: Fail})
		report.Summary = notReadySummary
		return report
	}
	version, err := s.ADB.Version(ctx)
	if err != nil || strings.TrimSpace(version) == "" {
		report.Checks = append(report.Checks, Check{Name: "adb", Detail: "not available", State: Fail})
		report.Summary = notReadySummary
		return report
	}
	report.Checks = append(report.Checks, Check{Name: "adb", Detail: firstLine(version), State: Pass})

	if selector == "" {
		selector = cfg.Connection.Device
	}
	target, err := adb.Discover(ctx, s.ADB, adb.SelectionOptions{PreferUSB: cfg.Connection.PreferUSB, Target: selector})
	if err != nil {
		report.Checks = append(report.Checks, Check{Name: "Galaxy", Detail: discoveryDetail(err), State: Fail})
		report.Summary = notReadySummary
		s.addOptionalHostChecks(ctx, &report)
		return report
	}
	info := target.Info()
	report.Checks = append(report.Checks,
		Check{Name: "Galaxy", Detail: info.Model, State: Pass},
		Check{Name: "Connection", Detail: connectionLabel(info.Connection), State: Pass},
		Check{Name: "Samsung Messages", Detail: "com.samsung.android.messaging", State: Pass},
	)

	store := provider.NewStore(target)
	requiredReady := true
	for _, check := range []struct {
		name string
		uri  string
	}{
		{name: "SMS Provider", uri: "content://sms"},
		{name: "Conversations", uri: "content://mms-sms/conversations?simple=true"},
		{name: "Contacts Provider", uri: "content://com.android.contacts/data/phones"},
	} {
		if _, err := store.Probe(ctx, check.uri, []string{"_id"}); err != nil {
			report.Checks = append(report.Checks, Check{Name: check.name, Detail: providerDetail(err), State: Fail})
			requiredReady = false
		} else {
			report.Checks = append(report.Checks, Check{Name: check.name, Detail: "accessible", State: Pass})
		}
	}

	for _, check := range []struct {
		name       string
		uri        string
		projection []string
		detail     string
	}{
		{name: "MMS Provider", uri: "content://mms", projection: []string{"_id"}, detail: "accessible"},
		{name: "MMS Parts", uri: "content://mms/part", projection: []string{"_id"}, detail: "accessible"},
		{name: "RCS", uri: "content://sms", projection: []string{"_id", "teleservice_id", "app_id", "chat_type", "correlation_tag"}, detail: "standard extension columns accessible; mapping deferred"},
	} {
		if _, err := store.Probe(ctx, check.uri, check.projection); err != nil {
			report.Checks = append(report.Checks, Check{Name: check.name, Detail: "unavailable or unsupported", State: Info})
		} else {
			report.Checks = append(report.Checks, Check{Name: check.name, Detail: check.detail, State: Info})
		}
	}

	report.Ready = requiredReady
	if report.Ready {
		report.Summary = "Read mode ready. Sending not implemented yet."
	} else {
		report.Summary = notReadySummary
	}
	s.addOptionalHostChecks(ctx, &report)
	return report
}

const notReadySummary = "Read mode not ready. Enable USB debugging or connect an already-paired Wireless Debugging device."

func (s Service) addOptionalHostChecks(ctx context.Context, report *Report) {
	if _, err := s.ADB.MDNSServices(ctx); err != nil {
		report.Checks = append(report.Checks, Check{Name: "Wireless discovery", Detail: "unavailable", State: Info})
	} else {
		report.Checks = append(report.Checks, Check{Name: "Wireless discovery", Detail: "available", State: Info})
	}
	if s.InspectScrcpy == nil {
		report.Checks = append(report.Checks, Check{Name: "scrcpy", Detail: "not inspected", State: Info})
		return
	}
	info, err := s.InspectScrcpy(ctx, "")
	if err != nil {
		report.Checks = append(report.Checks, Check{Name: "scrcpy", Detail: "not installed", State: Info})
		return
	}
	report.Checks = append(report.Checks, Check{Name: "scrcpy", Detail: info.Version, State: Pass})
}

func firstLine(value string) string {
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return strings.TrimSpace(value)
}

func connectionLabel(connection domain.ConnectionKind) string {
	if connection == domain.ConnectionWireless {
		return "Wireless"
	}
	return "USB"
}

func discoveryDetail(err error) string {
	switch {
	case errors.Is(err, adb.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, adb.ErrOffline):
		return "offline"
	case errors.Is(err, adb.ErrMultipleDevices):
		return "multiple eligible devices; use --device"
	case errors.Is(err, adb.ErrSamsungMessagesNotInstalled):
		return "Samsung Messages not installed"
	default:
		return "no authorized Samsung Galaxy found"
	}
}

func providerDetail(err error) string {
	if errors.Is(err, provider.ErrProviderPermissionDenied) {
		return "permission denied"
	}
	return "unavailable or malformed output"
}
