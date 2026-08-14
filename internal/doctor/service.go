package doctor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/galaxytty/galaxytty/internal/adb"
	"github.com/galaxytty/galaxytty/internal/config"
	"github.com/galaxytty/galaxytty/internal/domain"
	"github.com/galaxytty/galaxytty/internal/provider"
	"github.com/galaxytty/galaxytty/internal/samsung"
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
	LookPath      func(string) (string, error)
}

func Run(ctx context.Context, cfg config.Config, selector string) Report {
	client, err := adb.NewClient("", 5*time.Second)
	if err != nil {
		report := Report{
			Checks: []Check{{Name: "adb", Detail: "not found", State: Fail}},
		}
		setReadiness(&report, false, false)
		return report
	}
	return (Service{ADB: client, InspectScrcpy: scrcpy.Inspect}).Run(ctx, cfg, selector)
}

func (s Service) Run(ctx context.Context, cfg config.Config, selector string) Report {
	report := Report{}
	if s.ADB == nil {
		report.Checks = append(report.Checks, Check{Name: "adb", Detail: "not available", State: Fail})
		setReadiness(&report, false, false)
		return report
	}
	version, err := s.ADB.Version(ctx)
	if err != nil || strings.TrimSpace(version) == "" {
		report.Checks = append(report.Checks, Check{Name: "adb", Detail: "not available", State: Fail})
		s.finish(ctx, cfg, &report, false, false)
		return report
	}
	report.Checks = append(report.Checks, Check{Name: "adb", Detail: firstLine(version), State: Pass})

	if selector == "" {
		selector = cfg.Connection.Device
	}
	target, err := adb.Discover(ctx, s.ADB, adb.SelectionOptions{PreferUSB: cfg.Connection.PreferUSB, Target: selector})
	if err != nil {
		report.Checks = append(report.Checks, Check{Name: "Galaxy", Detail: discoveryDetail(err), State: Fail})
		s.finish(ctx, cfg, &report, false, false)
		return report
	}
	info := target.Info()
	report.Checks = append(report.Checks,
		Check{Name: "Galaxy", Detail: info.Model, State: Pass},
		Check{Name: "Connection", Detail: connectionLabel(info.Connection), State: Pass},
		Check{Name: "Samsung Messages installed", Detail: samsung.MessagesPackage, State: Pass},
	)
	defaultHandlerReady := true
	if err := samsung.CheckDefaultSMSHandler(ctx, target); err != nil {
		report.Checks = append(report.Checks, Check{Name: "Samsung Messages default SMS handler", Detail: "installed but not default", State: Fail})
		defaultHandlerReady = false
	} else {
		report.Checks = append(report.Checks, Check{Name: "Samsung Messages default SMS handler", Detail: "default", State: Pass})
	}

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

	s.finish(ctx, cfg, &report, requiredReady, defaultHandlerReady)
	return report
}

func (s Service) finish(ctx context.Context, cfg config.Config, report *Report, readReady, defaultHandlerReady bool) {
	if _, err := s.ADB.MDNSServices(ctx); err != nil {
		report.Checks = append(report.Checks, Check{Name: "Wireless discovery", Detail: "unavailable", State: Info})
	} else {
		report.Checks = append(report.Checks, Check{Name: "Wireless discovery", Detail: "available", State: Info})
	}

	sendPrerequisites := defaultHandlerReady
	if s.InspectScrcpy == nil {
		s.InspectScrcpy = scrcpy.Inspect
	}
	info, err := s.InspectScrcpy(ctx, "")
	if err != nil {
		report.Checks = append(report.Checks, Check{Name: "scrcpy", Detail: "not installed", State: Fail})
		sendPrerequisites = false
	} else {
		report.Checks = append(report.Checks, Check{Name: "scrcpy", Detail: info.Version, State: Pass})
	}

	lookPath := s.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	clipboardReady := true
	for _, name := range []string{"pbcopy", "pbpaste", "osascript"} {
		if _, err := lookPath(name); err != nil {
			clipboardReady = false
		}
	}
	if clipboardReady {
		report.Checks = append(report.Checks, Check{Name: "macOS clipboard compatibility", Detail: "pbcopy, pbpaste, and scrcpy shortcut bridge available", State: Pass})
	} else if cfg.Samsung.TextInputMode == domain.TextInputClipboard {
		report.Checks = append(report.Checks, Check{Name: "macOS clipboard compatibility", Detail: "required tools unavailable for clipboard mode", State: Fail})
		sendPrerequisites = false
	} else {
		report.Checks = append(report.Checks, Check{Name: "macOS clipboard compatibility", Detail: "unavailable; intent-body send remains supported", State: Info})
	}
	report.Checks = append(report.Checks, Check{Name: "Text input mode", Detail: string(cfg.Samsung.TextInputMode), State: Pass})

	layout := samsung.Layout{
		Width: cfg.Samsung.DisplayWidth, Height: cfg.Samsung.DisplayHeight,
		Composer: samsung.Point{X: cfg.Samsung.Layout.ComposerX, Y: cfg.Samsung.Layout.ComposerY},
		Send:     samsung.Point{X: cfg.Samsung.Layout.SendX, Y: cfg.Samsung.Layout.SendY},
	}
	if err := layout.Validate(); err != nil || cfg.Samsung.ClipboardSyncDelay.Duration <= 0 || cfg.Samsung.SendSettleDelay.Duration <= 0 || cfg.Samsung.VerificationTimeout.Duration <= 0 {
		report.Checks = append(report.Checks, Check{Name: "Virtual Display prerequisites", Detail: "invalid Samsung layout or timing", State: Fail})
		sendPrerequisites = false
	} else {
		report.Checks = append(report.Checks, Check{Name: "Virtual Display prerequisites", Detail: fmt.Sprintf("%dx%d layout configured", layout.Width, layout.Height), State: Pass})
	}
	setReadiness(report, readReady, readReady && sendPrerequisites)
}

func setReadiness(report *Report, readReady, sendReady bool) {
	report.ReadReady = readReady
	report.SendReady = sendReady
	report.Ready = readReady
	readLabel, sendLabel := "not ready", "not ready"
	if readReady {
		readLabel = "ready"
	}
	if sendReady {
		sendLabel = "ready"
	}
	report.Summary = fmt.Sprintf("Read: %s\nSend: %s", readLabel, sendLabel)
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
