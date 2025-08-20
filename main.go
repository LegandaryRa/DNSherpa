package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"go.etcd.io/etcd/clientv3"
)

type Config struct {
	EtcdEndpoints []string
	EtcdPrefix    string
	EtcdTLS       bool
	EtcdCertFile  string
	EtcdKeyFile   string
	EtcdCAFile    string
	DNSTarget     string
	RecordTTL     int
}

type DNSAutomator struct {
	dockerClient *client.Client
	etcdClient   *clientv3.Client
	config       Config
}

type DNSRecord struct {
	Host string `json:"host,omitempty"`
	TTL  int    `json:"ttl"`
}

func detectDNSTarget() string {
	// First check if DNS_TARGET is explicitly set
	if target := getEnv("DNS_TARGET", ""); target != "" {
		log.Printf("Using DNS_TARGET from environment: %s", target)
		return target
	}
	
	// Try to read hostname from mounted /etc/hostname
	data, err := ioutil.ReadFile("/host/hostname")
	if err != nil {
		log.Fatalf("FATAL: Cannot read /host/hostname - Please mount host's /etc/hostname with: -v /etc/hostname:/host/hostname:ro")
	}
	
	hostname := strings.TrimSpace(string(data))
	if hostname == "" {
		log.Fatalf("FATAL: /host/hostname is empty - Host's /etc/hostname appears to be empty")
	}
	
	// Check if hostname from file is already FQDN
	if strings.Contains(hostname, ".") {
		log.Printf("Detected DNS target from /host/hostname: %s", hostname)
		return hostname
	}
	
	// Hostname is not FQDN, check if DOMAIN is set
	domain := getEnv("DOMAIN", "")
	if domain == "" {
		log.Fatalf("FATAL: Host hostname '%s' is not FQDN and DOMAIN environment variable is not set. Please set DOMAIN=your-domain.com", hostname)
	}
	
	fqdn := hostname + "." + domain
	log.Printf("Detected DNS target from /host/hostname + domain: %s", fqdn)
	return fqdn
}


func loadConfig() Config {
	etcdEndpoints := strings.Split(getEnv("ETCD_ENDPOINTS", "172.16.0.221:2379,172.16.0.222:2379"), ",")
	
	// Parse TLS setting
	etcdTLS, _ := strconv.ParseBool(getEnv("ETCD_TLS", "false"))
	
	return Config{
		EtcdEndpoints: etcdEndpoints,
		EtcdPrefix:    getEnv("ETCD_PREFIX", "/skydns"),
		EtcdTLS:       etcdTLS,
		EtcdCertFile:  getEnv("ETCD_CERT_FILE", ""),
		EtcdKeyFile:   getEnv("ETCD_KEY_FILE", ""),
		EtcdCAFile:    getEnv("ETCD_CA_FILE", ""),
		DNSTarget:     detectDNSTarget(),
		RecordTTL:     300,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func NewDNSAutomator() (*DNSAutomator, error) {
	config := loadConfig()
	
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Build etcd client config
	etcdConfig := clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: 5 * time.Second,
	}

	// Note: Authentication is handled per-request in etcd v3

	// Add TLS configuration if enabled
	if config.EtcdTLS {
		tlsConfig, err := buildTLSConfig(config)
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS config: %w", err)
		}
		etcdConfig.TLS = tlsConfig
	}

	etcdClient, err := clientv3.New(etcdConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	return &DNSAutomator{
		dockerClient: dockerClient,
		etcdClient:   etcdClient,
		config:       config,
	}, nil
}

func buildTLSConfig(config Config) (*tls.Config, error) {
	tlsConfig := &tls.Config{}

	// Load CA certificate if provided
	if config.EtcdCAFile != "" {
		caCert, err := ioutil.ReadFile(config.EtcdCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate and key if provided
	if config.EtcdCertFile != "" && config.EtcdKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.EtcdCertFile, config.EtcdKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}


func (da *DNSAutomator) extractHostsFromLabels(labels map[string]string) []string {
	var hosts []string
	hostRegex := regexp.MustCompile(`Host\(\s*\x60([^` + "`" + `]+)\x60\s*\)`)
	
	for key, value := range labels {
		if strings.Contains(key, "traefik.http.routers.") && strings.Contains(key, ".rule") {
			matches := hostRegex.FindAllStringSubmatch(value, -1)
			for _, match := range matches {
				if len(match) > 1 {
					hosts = append(hosts, match[1])
				}
			}
		}
	}
	
	return hosts
}

func (da *DNSAutomator) createDNSRecord(hostname string) error {
	parts := strings.Split(hostname, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	key := fmt.Sprintf("%s/%s", da.config.EtcdPrefix, strings.Join(parts, "/"))
	
	var record DNSRecord
	target := da.config.DNSTarget
	
	// Check if target is an IP address
	if ip := net.ParseIP(target); ip != nil {
		// Create A or AAAA record for IP
		record = DNSRecord{
			Host: target,
			TTL:  da.config.RecordTTL,
		}
		if ip.To4() != nil {
			log.Printf("Creating A record: %s -> %s", hostname, target)
		} else {
			log.Printf("Creating AAAA record: %s -> %s", hostname, target)
		}
	} else {
		// Create CNAME record for hostname
		record = DNSRecord{
			Host: target,
			TTL:  da.config.RecordTTL,
		}
		log.Printf("Creating CNAME record: %s -> %s", hostname, target)
	}
	
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal DNS record: %w", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err = da.etcdClient.Put(ctx, key, string(recordJSON))
	if err != nil {
		return fmt.Errorf("failed to create DNS record for %s: %w", hostname, err)
	}
	
	return nil
}


func (da *DNSAutomator) handleContainerEvent(event events.Message) {
	if event.Type != events.ContainerEventType {
		return
	}

	// Only handle container start events
	if event.Action != "start" {
		return
	}

	containerID := event.ID
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	container, err := da.dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		log.Printf("Failed to inspect container %s: %v", containerID, err)
		return
	}
	
	hosts := da.extractHostsFromLabels(container.Config.Labels)
	if len(hosts) == 0 {
		return
	}
	
	// Create DNS records for all hosts
	for _, host := range hosts {
		if err := da.createDNSRecord(host); err != nil {
			log.Printf("Error creating DNS record for %s: %v", host, err)
		}
	}
}

func (da *DNSAutomator) syncExistingContainers() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	containers, err := da.dockerClient.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	
	log.Printf("Syncing %d existing containers", len(containers))
	
	for _, container := range containers {
		hosts := da.extractHostsFromLabels(container.Labels)
		for _, host := range hosts {
			if err := da.createDNSRecord(host); err != nil {
				log.Printf("Error creating DNS record for %s: %v", host, err)
			}
		}
	}
	
	return nil
}

func (da *DNSAutomator) Start() error {
	log.Println("Starting Traefik DNS Automator...")
	
	if err := da.syncExistingContainers(); err != nil {
		log.Printf("Warning: Failed to sync existing containers: %v", err)
	}
	
	ctx := context.Background()
	eventChan, errChan := da.dockerClient.Events(ctx, events.ListOptions{})
	
	log.Println("Listening for Docker events...")
	
	for {
		select {
		case event := <-eventChan:
			da.handleContainerEvent(event)
		case err := <-errChan:
			if err != nil {
				log.Printf("Docker events error: %v", err)
				return err
			}
		}
	}
}

func (da *DNSAutomator) Close() {
	if da.dockerClient != nil {
		da.dockerClient.Close()
	}
	if da.etcdClient != nil {
		da.etcdClient.Close()
	}
}

func main() {
	automator, err := NewDNSAutomator()
	if err != nil {
		log.Fatalf("Failed to create DNS automator: %v", err)
	}
	defer automator.Close()
	
	if err := automator.Start(); err != nil {
		log.Fatalf("DNS automator failed: %v", err)
	}
}