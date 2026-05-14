package config

import (
	"charm.land/catwalk/pkg/catwalk"
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/xiehqing/hiagent-core/internal/home"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

const customProviderName = "custom_providers"
const openProviderName = "open_provider"
const EnvLocalAppData = "LOCALAPPDATA"
const EnvOpenProviderData = "OPEN_PROVIDER_DATA"
const EnvCustomProviderData = "CUSTOM_PROVIDER_DATA"
const EnvUserProfile = "USERPROFILE"

// OpenProviderData 返回开放模型提供者
func OpenProviderData() string {
	if openProviderData := os.Getenv(EnvOpenProviderData); openProviderData != "" {
		return filepath.Join(openProviderData, fmt.Sprintf("%s.json", openProviderName))
	}
	if runtime.GOOS == "windows" {
		localAppData := cmp.Or(os.Getenv(EnvLocalAppData),
			filepath.Join(os.Getenv(EnvUserProfile), "AppData", "Local"))
		return filepath.Join(localAppData, appName, fmt.Sprintf("%s.json", openProviderName))
	}
	return filepath.Join(home.Dir(), ".local", "share", appName, fmt.Sprintf("%s.json", openProviderName))
}

// CustomProviderData 返回开放模型提供者
func CustomProviderData() string {
	if openProviderData := os.Getenv(EnvCustomProviderData); openProviderData != "" {
		return filepath.Join(openProviderData, fmt.Sprintf("%s.json", customProviderName))
	}
	if runtime.GOOS == "windows" {
		localAppData := cmp.Or(os.Getenv(EnvLocalAppData),
			filepath.Join(os.Getenv(EnvUserProfile), "AppData", "Local"))
		return filepath.Join(localAppData, appName, fmt.Sprintf("%s.json", customProviderName))
	}
	return filepath.Join(home.Dir(), ".local", "share", appName, fmt.Sprintf("%s.json", customProviderName))
}

// CustomProviders 返回开放模型提供者
func CustomProviders() ([]catwalk.Provider, string, error) {
	customProviderFile := CustomProviderData()
	var allProviders = make([]catwalk.Provider, 0)
	if customProviderFile != "" {
		if _, err := os.Stat(customProviderFile); err != nil && os.IsNotExist(err) {
			slog.Warn("No Custom Provider provider file found", "file", customProviderFile)
		} else {
			slog.Info("Custom Provider provider", "file", customProviderFile)
			bytes, err := os.ReadFile(customProviderFile)
			if err != nil {
				slog.Warn("Failed to read Custom Provider provider file", "err", err)
			} else {
				var openProviders []catwalk.Provider
				if err = json.Unmarshal(bytes, &openProviders); err != nil {
					slog.Error("failed to unmarshal Custom Provider provider file", "err", err)
				} else {
					allProviders = append(allProviders, openProviders...)
				}
			}
		}
	} else {
		slog.Warn("No Custom Provider provider file found")
	}
	return allProviders, customProviderFile, nil
}

// OpenProviders 返回开放模型提供者
func OpenProviders() ([]catwalk.Provider, string, error) {
	openProviderFile := OpenProviderData()
	var allProviders = make([]catwalk.Provider, 0)
	if openProviderFile != "" {
		if _, err := os.Stat(openProviderFile); err != nil && os.IsNotExist(err) {
			slog.Warn("No Open Provider provider file found", "file", openProviderFile)
		} else {
			slog.Info("Open Provider provider", "file", openProviderFile)
			bytes, err := os.ReadFile(openProviderFile)
			if err != nil {
				slog.Warn("Failed to read Open Provider provider file", "err", err)
			} else {
				var openProviders []catwalk.Provider
				if err = json.Unmarshal(bytes, &openProviders); err != nil {
					slog.Error("Failed to unmarshal Open Provider provider file", "err", err)
				} else {
					allProviders = append(allProviders, openProviders...)
				}
			}
		}
	} else {
		slog.Warn("no Open Provider provider file found")
	}
	return allProviders, openProviderFile, nil
}
