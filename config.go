package main

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// etcd configuration
	EtcdEndpoints []string
	EtcdPrefix    string
	EtcdTLS       bool
	EtcdCertFile  string
	EtcdKeyFile   string
	EtcdCAFile    string
	
	// DNS configuration
	DNSTarget     string
	RecordTTL     int
	Domain        string
	
	// Agent mode
	AgentMode     string
	
	// Proxmox configuration
	ProxmoxAPIURL        string
	ProxmoxTokenID       string
	ProxmoxTokenSecret   string
	ProxmoxPollInterval  time.Duration
	ProxmoxVerifySSL     bool
	ProxmoxInterface     string
	ProxmoxMultiIPv4     string
}

func LoadConfig() Config {
	etcdEndpoints := strings.Split(getEnv("ETCD_ENDPOINTS", "172.16.0.221:2379,172.16.0.222:2379"), ",")
	
	// Parse TLS setting
	etcdTLS, _ := strconv.ParseBool(getEnv("ETCD_TLS", "false"))
	
	// Parse Proxmox settings
	proxmoxVerifySSL, _ := strconv.ParseBool(getEnv("PROXMOX_VERIFY_SSL", "false"))
	proxmoxPollInterval, _ := time.ParseDuration(getEnv("PROXMOX_POLL_INTERVAL", "30s"))
	
	return Config{
		// etcd configuration
		EtcdEndpoints: etcdEndpoints,
		EtcdPrefix:    getEnv("ETCD_PREFIX", "/skydns"),
		EtcdTLS:       etcdTLS,
		EtcdCertFile:  getEnv("ETCD_CERT_FILE", ""),
		EtcdKeyFile:   getEnv("ETCD_KEY_FILE", ""),
		EtcdCAFile:    getEnv("ETCD_CA_FILE", ""),
		
		// DNS configuration
		DNSTarget:     detectDNSTarget(),
		RecordTTL:     300,
		Domain:        getEnv("DOMAIN", ""),
		
		// Agent mode
		AgentMode:     getEnv("AGENT_MODE", "docker"),
		
		// Proxmox configuration
		ProxmoxAPIURL:        getEnv("PROXMOX_API_URL", ""),
		ProxmoxTokenID:       getEnv("PROXMOX_TOKEN_ID", ""),
		ProxmoxTokenSecret:   getEnv("PROXMOX_TOKEN_SECRET", ""),
		ProxmoxPollInterval:  proxmoxPollInterval,
		ProxmoxVerifySSL:     proxmoxVerifySSL,
		ProxmoxInterface:     getEnv("PROXMOX_INTERFACE", "eth0"),
		ProxmoxMultiIPv4:     getEnv("PROXMOX_MULTI_IPV4", "first"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func detectDNSTarget() string {
	// First check if DNS_TARGET is explicitly set
	if target := getEnv("DNS_TARGET", ""); target != "" {
		if log != nil {
			log.WithField("target", target).Info("Using DNS_TARGET from environment")
		}
		return target
	}
	
	// Try to read hostname from mounted /etc/hostname
	data, err := ioutil.ReadFile("/host/hostname")
	if err != nil {
		if log != nil {
			log.WithError(err).Fatal("Cannot read /host/hostname - Please mount host's /etc/hostname with: -v /etc/hostname:/host/hostname:ro")
		} else {
			panic("FATAL: Cannot read /host/hostname - Please mount host's /etc/hostname with: -v /etc/hostname:/host/hostname:ro")
		}
	}
	
	hostname := strings.TrimSpace(string(data))
	if hostname == "" {
		if log != nil {
			log.Fatal("/host/hostname is empty - Host's /etc/hostname appears to be empty")
		} else {
			panic("FATAL: /host/hostname is empty - Host's /etc/hostname appears to be empty")
		}
	}
	
	// Check if hostname from file is already FQDN
	if strings.Contains(hostname, ".") {
		if log != nil {
			log.WithField("target", hostname).Info("Detected DNS target from /host/hostname")
		}
		return hostname
	}
	
	// Hostname is not FQDN, check if DOMAIN is set
	domain := getEnv("DOMAIN", "")
	if domain == "" {
		if log != nil {
			log.WithField("hostname", hostname).Fatal("Host hostname is not FQDN and DOMAIN environment variable is not set. Please set DOMAIN=your-domain.com")
		} else {
			panic("FATAL: Host hostname is not FQDN and DOMAIN environment variable is not set. Please set DOMAIN=your-domain.com")
		}
	}
	
	fqdn := hostname + "." + domain
	if log != nil {
		log.WithFields(map[string]interface{}{
			"hostname": hostname,
			"domain":   domain,
			"fqdn":     fqdn,
		}).Info("Detected DNS target from hostname and domain")
	}
	return fqdn
}