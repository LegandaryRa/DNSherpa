package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

var log *logrus.Logger

// InitializeLogger sets up the global logger with configuration from environment variables
func InitializeLogger() {
	log = logrus.New()
	
	// Configure log level
	levelStr := strings.ToLower(getEnv("LOG_LEVEL", "info"))
	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		level = logrus.InfoLevel
		log.Warnf("Invalid LOG_LEVEL '%s', defaulting to 'info'", levelStr)
	}
	log.SetLevel(level)
	
	// Configure log format
	formatStr := strings.ToLower(getEnv("LOG_FORMAT", "text"))
	switch formatStr {
	case "json":
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	case "text":
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			DisableColors:   false,
		})
	default:
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			DisableColors:   false,
		})
		log.Warnf("Invalid LOG_FORMAT '%s', defaulting to 'text'", formatStr)
	}
	
	// Output to stdout
	log.SetOutput(os.Stdout)
}

// ShowStartupBanner displays the application banner and version information
func ShowStartupBanner() {
	versionInfo := GetVersionInfo()
	
	// Create banner with dynamic version
	banner := fmt.Sprintf(`╭─────────────────────────────────────────────────────────╮
│                   🚀 DNSherpa %s%s│
│          Automatic DNS Management for Docker & Proxmox │
╰─────────────────────────────────────────────────────────╯`, 
		versionInfo.Version,
		strings.Repeat(" ", max(0, 25-len(versionInfo.Version)))) // Pad to align
	
	log.Info(banner)
	
	// Log detailed version information
	logFields := map[string]interface{}{
		"version":    versionInfo.Version,
		"go_version": versionInfo.GoVersion,
		"platform":   versionInfo.Platform,
		"log_level":  log.Level.String(),
	}
	
	// Add development build info if available
	if IsDevelopmentBuild() && versionInfo.GitCommit != "unknown" {
		logFields["git_commit"] = versionInfo.GitCommit
		logFields["git_branch"] = versionInfo.GitBranch
		if versionInfo.BuildTime != "unknown" {
			logFields["build_time"] = versionInfo.BuildTime
		}
	}
	
	log.WithFields(logFields).Info("Application starting")
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// LogConfigurationSummary displays the current configuration without sensitive data
func LogConfigurationSummary(config Config) {
	log.WithFields(logrus.Fields{
		"agent_mode":         config.AgentMode,
		"etcd_endpoints":     config.EtcdEndpoints,
		"etcd_prefix":        config.EtcdPrefix,
		"etcd_tls":          config.EtcdTLS,
		"dns_target":        config.DNSTarget,
		"domain":            config.Domain,
		"record_ttl":        config.RecordTTL,
	}).Info("Configuration loaded")
	
	// Log Proxmox-specific config if relevant
	if config.AgentMode == "proxmox" || config.AgentMode == "hybrid" {
		if config.ProxmoxAPIURL != "" {
			log.WithFields(logrus.Fields{
				"api_url":          config.ProxmoxAPIURL,
				"verify_ssl":       config.ProxmoxVerifySSL,
				"poll_interval":    config.ProxmoxPollInterval,
				"interface":        config.ProxmoxInterface,
				"multi_ipv4":       config.ProxmoxMultiIPv4,
				"token_configured": config.ProxmoxTokenID != "" && config.ProxmoxTokenSecret != "",
			}).Info("Proxmox configuration loaded")
		} else {
			log.Warn("Proxmox mode enabled but no API URL configured")
		}
	}
}