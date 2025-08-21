package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"time"

	"go.etcd.io/etcd/clientv3"
)

type DNSRecord struct {
	Host string `json:"host,omitempty"`
	TTL  int    `json:"ttl"`
}

type EtcdClient struct {
	client *clientv3.Client
	config Config
}

func NewEtcdClient(config Config) (*EtcdClient, error) {
	// Build etcd client config
	etcdConfig := clientv3.Config{
		Endpoints:   config.EtcdEndpoints,
		DialTimeout: 5 * time.Second,
	}

	// Add TLS configuration if enabled
	if config.EtcdTLS {
		tlsConfig, err := buildTLSConfig(config)
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS config: %w", err)
		}
		etcdConfig.TLS = tlsConfig
	}

	client, err := clientv3.New(etcdConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	return &EtcdClient{
		client: client,
		config: config,
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

func (ec *EtcdClient) CreateDNSRecord(hostname string) error {
	parts := strings.Split(hostname, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	key := fmt.Sprintf("%s/%s", ec.config.EtcdPrefix, strings.Join(parts, "/"))
	
	var record DNSRecord
	target := ec.config.DNSTarget
	
	// Check if target is an IP address
	if ip := net.ParseIP(target); ip != nil {
		// Create A or AAAA record for IP
		record = DNSRecord{
			Host: target,
			TTL:  ec.config.RecordTTL,
		}
		if ip.To4() != nil {
			log.WithFields(map[string]interface{}{
				"hostname": hostname,
				"target":   target,
				"type":     "A",
			}).Info("Creating DNS record")
		} else {
			log.WithFields(map[string]interface{}{
				"hostname": hostname,
				"target":   target,
				"type":     "AAAA",
			}).Info("Creating DNS record")
		}
	} else {
		// Create CNAME record for hostname
		record = DNSRecord{
			Host: target,
			TTL:  ec.config.RecordTTL,
		}
		log.WithFields(map[string]interface{}{
			"hostname": hostname,
			"target":   target,
			"type":     "CNAME",
		}).Info("Creating DNS record")
	}
	
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal DNS record: %w", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	_, err = ec.client.Put(ctx, key, string(recordJSON))
	if err != nil {
		return fmt.Errorf("failed to create DNS record for %s: %w", hostname, err)
	}
	
	return nil
}

func (ec *EtcdClient) CreateDNSRecords(hostname string, ips []string) error {
	parts := strings.Split(hostname, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	basePath := fmt.Sprintf("%s/%s", ec.config.EtcdPrefix, strings.Join(parts, "/"))
	
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	
	var ipv4Count, ipv6Count int
	var createdRecords []string
	
	for _, ip := range ips {
		var key string
		var recordType string
		
		if netIP := net.ParseIP(ip); netIP != nil {
			if netIP.To4() != nil {
				// IPv4 - A record
				ipv4Count++
				key = fmt.Sprintf("%s/a%d", basePath, ipv4Count)
				recordType = "A"
			} else {
				// IPv6 - AAAA record  
				ipv6Count++
				key = fmt.Sprintf("%s/aaaa%d", basePath, ipv6Count)
				recordType = "AAAA"
			}
			
			record := DNSRecord{Host: ip, TTL: ec.config.RecordTTL}
			recordJSON, err := json.Marshal(record)
			if err != nil {
				return fmt.Errorf("failed to marshal DNS record: %w", err)
			}
			
			_, err = ec.client.Put(ctx, key, string(recordJSON))
			if err != nil {
				return fmt.Errorf("failed to create %s record for %s: %w", recordType, hostname, err)
			}
			
			createdRecords = append(createdRecords, fmt.Sprintf("%s->%s", recordType, ip))
			log.WithFields(map[string]interface{}{
				"hostname": hostname,
				"ip":       ip,
				"type":     recordType,
			}).Info("Created DNS record")
		}
	}
	
	if len(createdRecords) > 0 {
		log.WithFields(map[string]interface{}{
			"hostname":     hostname,
			"record_count": len(createdRecords),
			"records":      strings.Join(createdRecords, ", "),
		}).Info("DNS records created successfully")
	}
	
	return nil
}

func (ec *EtcdClient) Close() {
	if ec.client != nil {
		ec.client.Close()
	}
}