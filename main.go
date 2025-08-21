package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

type DNSAutomator struct {
	dockerClient *DockerClient
	proxmoxClient *ProxmoxClient
	etcdClient   *EtcdClient
	config       Config
}


func NewDNSAutomator() (*DNSAutomator, error) {
	config := LoadConfig()
	
	etcdClient, err := NewEtcdClient(config)
	if err != nil {
		return nil, err
	}

	dockerClient, err := NewDockerClient(etcdClient)
	if err != nil {
		return nil, err
	}

	proxmoxClient, err := NewProxmoxClient(etcdClient, config)
	if err != nil {
		return nil, err
	}

	return &DNSAutomator{
		dockerClient:  dockerClient,
		proxmoxClient: proxmoxClient,
		etcdClient:    etcdClient,
		config:        config,
	}, nil
}


func (da *DNSAutomator) Start() error {
	log.WithField("mode", da.config.AgentMode).Info("Starting DNSherpa")
	
	ctx := context.Background()
	
	// Start monitoring based on agent mode
	switch da.config.AgentMode {
	case "docker":
		log.Info("Starting Docker-only monitoring")
		return da.dockerClient.StartEventMonitoring(ctx)
		
	case "proxmox":
		log.Info("Starting Proxmox-only monitoring")
		return da.proxmoxClient.StartMonitoring(ctx)
		
	case "hybrid":
		log.Info("Starting hybrid monitoring (Docker + Proxmox)")
		
		// Start Docker monitoring
		go func() {
			if err := da.dockerClient.StartEventMonitoring(ctx); err != nil {
				log.WithError(err).Error("Docker monitoring failed")
			}
		}()
		
		// Start Proxmox monitoring
		go func() {
			if err := da.proxmoxClient.StartMonitoring(ctx); err != nil {
				log.WithError(err).Error("Proxmox monitoring failed")
			}
		}()
		
		// Block main thread
		<-ctx.Done()
		return ctx.Err()
		
	default:
		return fmt.Errorf("invalid agent mode: %s (valid options: docker, proxmox, hybrid)", da.config.AgentMode)
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
	// Initialize logging first
	InitializeLogger()
	
	// Show startup banner
	ShowStartupBanner()
	
	// Load and validate configuration
	config := LoadConfig()
	LogConfigurationSummary(config)
	
	// Create DNS automator
	log.Info("Initializing DNS automator...")
	automator, err := NewDNSAutomator()
	if err != nil {
		log.WithError(err).Fatal("Failed to create DNS automator. Check your configuration and network connectivity.")
	}
	defer automator.Close()
	
	log.Info("DNS automator initialized successfully")
	
	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Start the automator in a goroutine
	go func() {
		if err := automator.Start(); err != nil {
			log.WithError(err).Fatal("DNS automator failed")
		}
	}()
	
	// Wait for shutdown signal
	sig := <-sigChan
	log.WithField("signal", sig).Info("Received shutdown signal, stopping gracefully...")
}