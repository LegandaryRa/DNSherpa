package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"
)

type ProxmoxClient struct {
	client     *proxmox.Client
	etcdClient *EtcdClient
	config     Config
}

func NewProxmoxClient(etcdClient *EtcdClient, config Config) (*ProxmoxClient, error) {
	if config.ProxmoxAPIURL == "" {
		return &ProxmoxClient{
			etcdClient: etcdClient,
			config:     config,
		}, nil // Return empty client for non-proxmox modes
	}

	// Ensure API URL has the correct path
	apiURL := config.ProxmoxAPIURL
	if !strings.HasSuffix(apiURL, "/api2/json") {
		// Remove trailing slash if present, then add /api2/json
		apiURL = strings.TrimSuffix(apiURL, "/") + "/api2/json"
	}

	log.WithField("api_url", apiURL).Info("Connecting to Proxmox API")

	// Create HTTP client with SSL verification setting
	httpClient := &http.Client{}
	if !config.ProxmoxVerifySSL {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	// Create Proxmox client with API token
	client := proxmox.NewClient(apiURL,
		proxmox.WithHTTPClient(httpClient),
		proxmox.WithAPIToken(config.ProxmoxTokenID, config.ProxmoxTokenSecret),
	)

	return &ProxmoxClient{
		client:     client,
		etcdClient: etcdClient,
		config:     config,
	}, nil
}

func (pc *ProxmoxClient) StartMonitoring(ctx context.Context) error {
	if pc.client == nil {
		log.Info("Proxmox client not configured, skipping monitoring")
		<-ctx.Done()
		return ctx.Err()
	}

	log.WithField("poll_interval", pc.config.ProxmoxPollInterval).Info("Starting Proxmox monitoring")

	// Test connection
	if err := pc.testConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to Proxmox: %w", err)
	}

	// Initial sync
	if err := pc.syncAllResources(ctx); err != nil {
		log.WithError(err).Warn("Initial sync failed")
	}

	// Start polling loop
	ticker := time.NewTicker(pc.config.ProxmoxPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := pc.syncAllResources(ctx); err != nil {
				log.WithError(err).Error("Error during Proxmox sync")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (pc *ProxmoxClient) testConnection(ctx context.Context) error {
	log.Info("Testing Proxmox API connection...")
	
	version, err := pc.client.Version(ctx)
	if err != nil {
		return fmt.Errorf("API connection test failed: %w", err)
	}
	
	log.WithFields(map[string]interface{}{
		"version": version.Version,
		"release": version.Release,
	}).Info("Proxmox API connection successful")
	
	// Test cluster access
	_, err = pc.client.Cluster(ctx)
	if err != nil {
		log.WithError(err).Warn("Cannot access cluster info")
	} else {
		log.Info("Cluster access successful")
		
		// Test basic resource access
		nodes, err := pc.client.Nodes(ctx)
		if err != nil {
			log.WithError(err).Warn("Cannot list nodes")
		} else {
			log.WithField("node_count", len(nodes)).Info("Found nodes in cluster")
			for _, node := range nodes {
				log.WithFields(map[string]interface{}{
					"node_name": node.Node,
					"status":    node.Status,
					"type":      node.Type,
				}).Debug("Cluster node details")
			}
		}
	}
	
	return nil
}

func (pc *ProxmoxClient) syncAllResources(ctx context.Context) error {
	log.Info("Syncing Proxmox VMs and containers...")
	
	// Get list of nodes first
	nodes, err := pc.client.Nodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	var processedCount int
	var skippedCount int

	// Query each node for VMs and containers
	for _, nodeStatus := range nodes {
		if nodeStatus.Status != "online" {
			log.WithField("node", nodeStatus.Node).Warn("Skipping offline node")
			continue
		}

		log.WithField("node", nodeStatus.Node).Debug("Querying VMs on node")
		
		node, err := pc.client.Node(ctx, nodeStatus.Node)
		if err != nil {
			log.WithFields(map[string]interface{}{
				"node":  nodeStatus.Node,
				"error": err,
			}).Error("Failed to get node")
			continue
		}

		// Get QEMU VMs on this node
		vms, err := node.VirtualMachines(ctx)
		if err != nil {
			log.WithFields(map[string]interface{}{
				"node":  nodeStatus.Node,
				"error": err,
			}).Error("Failed to get VMs on node")
		} else {
			log.WithFields(map[string]interface{}{
				"node":     nodeStatus.Node,
				"vm_count": len(vms),
			}).Info("Found QEMU VMs on node")
			for _, vm := range vms {
				log.WithFields(map[string]interface{}{
					"vm_name": vm.Name,
					"vmid":    vm.VMID,
					"status":  vm.Status,
				}).Debug("Processing VM")
				
				if vm.Status != "running" {
					log.WithFields(map[string]interface{}{
						"vm_name": vm.Name,
						"status":  vm.Status,
					}).Debug("Skipping non-running VM")
					skippedCount++
					continue
				}

				if err := pc.processVM(ctx, vm, nodeStatus.Node); err != nil {
					log.WithFields(map[string]interface{}{
						"vm_name": vm.Name,
						"error":   err,
					}).Error("Error processing VM")
					continue
				}
				processedCount++
			}
		}

		// Get LXC containers on this node
		containers, err := node.Containers(ctx)
		if err != nil {
			log.WithFields(map[string]interface{}{
				"node":  nodeStatus.Node,
				"error": err,
			}).Error("Failed to get containers on node")
		} else {
			log.WithFields(map[string]interface{}{
				"node":            nodeStatus.Node,
				"container_count": len(containers),
			}).Info("Found LXC containers on node")
			for _, container := range containers {
				log.WithFields(map[string]interface{}{
					"container_name": container.Name,
					"vmid":           container.VMID,
					"status":         container.Status,
				}).Debug("Processing container")
				
				if container.Status != "running" {
					log.WithFields(map[string]interface{}{
						"container_name": container.Name,
						"status":         container.Status,
					}).Debug("Skipping non-running container")
					skippedCount++
					continue
				}

				if err := pc.processContainer(ctx, container, nodeStatus.Node); err != nil {
					log.WithFields(map[string]interface{}{
						"container_name": container.Name,
						"error":          err,
					}).Error("Error processing container")
					continue
				}
				processedCount++
			}
		}
	}

	log.WithFields(map[string]interface{}{
		"processed": processedCount,
		"skipped":   skippedCount,
	}).Info("Completed Proxmox resource sync")
	return nil
}

func (pc *ProxmoxClient) processVM(ctx context.Context, vm *proxmox.VirtualMachine, nodeName string) error {
	// Check for opt-out tag
	if vm.HasTag("dnsherpa-skip") {
		log.WithField("vm_name", vm.Name).Info("Skipping VM due to dnsherpa-skip tag")
		return nil
	}

	// Generate hostname
	hostname := pc.generateHostname(vm.Name)
	
	// Get VM tags safely - avoid SplitTags() due to potential nil pointer issues
	var vmTags []string
	if vm.Tags != "" {
		vmTags = strings.Split(vm.Tags, ";")
	}
	
	// Get IP addresses using a simulated cluster resource for compatibility
	fakeResource := &proxmox.ClusterResource{
		Name:   vm.Name,
		Type:   "qemu",
		Status: vm.Status,
		VMID:   uint64(vm.VMID),
		Node:   nodeName,
	}
	
	ips, err := pc.getResourceIPs(ctx, fakeResource, vmTags)
	if err != nil {
		return fmt.Errorf("failed to get IPs for VM %s: %w", vm.Name, err)
	}

	if len(ips) == 0 {
		log.WithField("vm_name", vm.Name).Warn("No IPs found for VM")
		return nil
	}

	// Create DNS records
	return pc.etcdClient.CreateDNSRecords(hostname, ips)
}

func (pc *ProxmoxClient) processContainer(ctx context.Context, container *proxmox.Container, nodeName string) error {
	// Check for opt-out tag
	if container.HasTag("dnsherpa-skip") {
		log.WithField("container_name", container.Name).Info("Skipping container due to dnsherpa-skip tag")
		return nil
	}

	// Generate hostname
	hostname := pc.generateHostname(container.Name)
	
	// Get container tags safely - avoid SplitTags() due to potential nil pointer issues
	var containerTags []string
	if container.Tags != "" {
		containerTags = strings.Split(container.Tags, ";")
	}
	
	// Get IP addresses using a simulated cluster resource for compatibility
	fakeResource := &proxmox.ClusterResource{
		Name:   container.Name,
		Type:   "lxc",
		Status: container.Status,
		VMID:   uint64(container.VMID),
		Node:   nodeName,
	}
	
	ips, err := pc.getResourceIPs(ctx, fakeResource, containerTags)
	if err != nil {
		return fmt.Errorf("failed to get IPs for container %s: %w", container.Name, err)
	}

	if len(ips) == 0 {
		log.WithField("container_name", container.Name).Warn("No IPs found for container")
		return nil
	}

	// Create DNS records
	return pc.etcdClient.CreateDNSRecords(hostname, ips)
}

func (pc *ProxmoxClient) processResource(ctx context.Context, resource *proxmox.ClusterResource) error {
	// Get the node
	node, err := pc.client.Node(ctx, resource.Node)
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", resource.Node, err)
	}

	var vmName string
	var hasDnsherpaSkip bool
	var vmTags []string

	if resource.Type == "qemu" {
		vm, err := node.VirtualMachine(ctx, int(resource.VMID))
		if err != nil {
			return fmt.Errorf("failed to get VM %d: %w", resource.VMID, err)
		}
		
		vmName = resource.Name
		hasDnsherpaSkip = vm.HasTag("dnsherpa-skip")
		
		// Get all tags by splitting manually
		vm.SplitTags()
		if vm.Tags != "" {
			vmTags = strings.Split(vm.Tags, ";")
		}
		
	} else if resource.Type == "lxc" {
		container, err := node.Container(ctx, int(resource.VMID))
		if err != nil {
			return fmt.Errorf("failed to get container %d: %w", resource.VMID, err)
		}
		
		vmName = resource.Name
		hasDnsherpaSkip = container.HasTag("dnsherpa-skip")
		
		// Get all tags by splitting manually
		container.SplitTags()
		if container.Tags != "" {
			vmTags = strings.Split(container.Tags, ";")
		}
		
	} else {
		return nil // Skip non-VM resources
	}

	// Check for opt-out tag
	if hasDnsherpaSkip {
		log.WithField("vm_name", vmName).Info("Skipping VM due to dnsherpa-skip tag")
		return nil
	}

	// Generate hostname
	hostname := pc.generateHostname(vmName)
	
	// Get IP addresses
	ips, err := pc.getResourceIPs(ctx, resource, vmTags)
	if err != nil {
		return fmt.Errorf("failed to get IPs for %s: %w", vmName, err)
	}

	if len(ips) == 0 {
		log.WithField("vm_name", vmName).Warn("No IPs found for VM")
		return nil
	}

	// Create DNS records
	return pc.etcdClient.CreateDNSRecords(hostname, ips)
}

func (pc *ProxmoxClient) hasTagInList(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.TrimSpace(t) == tag {
			return true
		}
	}
	return false
}

func (pc *ProxmoxClient) getTagValue(tags []string, prefix string) string {
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if strings.HasPrefix(t, prefix+":") {
			return strings.TrimPrefix(t, prefix+":")
		}
	}
	return ""
}

func (pc *ProxmoxClient) generateHostname(vmName string) string {
	hostname := vmName
	
	// If VM name is not FQDN and we have a domain, append it
	if !strings.Contains(hostname, ".") && pc.config.Domain != "" {
		hostname = hostname + "." + pc.config.Domain
	}
	
	return hostname
}

func (pc *ProxmoxClient) getResourceIPs(ctx context.Context, resource *proxmox.ClusterResource, tags []string) ([]string, error) {
	// Check for specific IP tag first (highest priority)
	if specificIPs := pc.getTagValue(tags, "dnsherpa-ip"); specificIPs != "" {
		ips := strings.Split(specificIPs, ",")
		var cleanIPs []string
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if net.ParseIP(ip) != nil {
				cleanIPs = append(cleanIPs, ip)
			}
		}
		return cleanIPs, nil
	}

	// Get interface name (per-VM tag > global config > default)
	interfaceName := pc.getTagValue(tags, "dnsherpa-interface")
	if interfaceName == "" {
		interfaceName = pc.config.ProxmoxInterface
	}

	// Get IP addresses from the specified interface
	return pc.extractIPsFromInterface(ctx, resource, interfaceName)
}

func (pc *ProxmoxClient) extractIPsFromInterface(ctx context.Context, resource *proxmox.ClusterResource, interfaceName string) ([]string, error) {
	node, err := pc.client.Node(ctx, resource.Node)
	if err != nil {
		return nil, err
	}

	var ips []string
	
	if resource.Type == "qemu" {
		vm, err := node.VirtualMachine(ctx, int(resource.VMID))
		if err != nil {
			return nil, err
		}
		
		// Try to get network interfaces from agent
		interfaces, err := vm.AgentGetNetworkIFaces(ctx)
		if err != nil {
			log.WithFields(map[string]interface{}{
				"vm_name": resource.Name,
				"error":   err,
			}).Debug("QEMU agent not available, falling back to config")
			return pc.extractIPsFromConfig(ctx, resource)
		}
		
		for _, iface := range interfaces {
			if iface.Name == interfaceName {
				for _, ipAddr := range iface.IPAddresses {
					if ipAddr.IPAddress != "127.0.0.1" && ipAddr.IPAddress != "::1" {
						ips = append(ips, ipAddr.IPAddress)
					}
				}
			}
		}
	} else if resource.Type == "lxc" {
		// For LXC containers, try to get network interfaces directly
		container, err := node.Container(ctx, int(resource.VMID))
		if err != nil {
			return nil, err
		}
		
		// Try to get container network interfaces
		interfaces, err := container.Interfaces(ctx)
		if err != nil {
			log.WithFields(map[string]interface{}{
				"container_name": resource.Name,
				"error":          err,
			}).Debug("Failed to get container interfaces, falling back to config")
			return pc.extractIPsFromConfig(ctx, resource)
		}
		
		log.WithFields(map[string]interface{}{
			"container_name":   resource.Name,
			"interface_count": len(interfaces),
		}).Debug("Found container network interfaces")
		for _, iface := range interfaces {
			log.WithFields(map[string]interface{}{
				"container_name": resource.Name,
				"interface":      iface.Name,
				"ipv4":           iface.Inet,
				"ipv6":           iface.Inet6,
			}).Trace("Container interface details")
			
			if iface.Name == interfaceName {
				// Extract IPv4 address (remove CIDR suffix)
				if iface.Inet != "" && iface.Inet != "127.0.0.1/8" {
					ipv4 := strings.Split(iface.Inet, "/")[0]
					log.WithFields(map[string]interface{}{
						"container_name": resource.Name,
						"ipv4":           ipv4,
					}).Debug("Found IPv4 address")
					ips = append(ips, ipv4)
				}
				
				// Extract IPv6 address (remove CIDR suffix) 
				if iface.Inet6 != "" && !strings.HasPrefix(iface.Inet6, "::1/") && !strings.HasPrefix(iface.Inet6, "fe80::") {
					ipv6 := strings.Split(iface.Inet6, "/")[0]
					log.WithFields(map[string]interface{}{
						"container_name": resource.Name,
						"ipv6":           ipv6,
					}).Debug("Found IPv6 address")
					ips = append(ips, ipv6)
				}
			}
		}
	}

	// Apply multi-IPv4 strategy
	return pc.applyMultiIPv4Strategy(ips), nil
}

func (pc *ProxmoxClient) extractIPsFromConfig(ctx context.Context, resource *proxmox.ClusterResource) ([]string, error) {
	// This is a simplified fallback implementation
	// In practice, you might want to parse actual network config or use DHCP reservations
	
	// For now, return empty - this will be improved based on actual Proxmox setup needs
	log.WithField("resource_name", resource.Name).Debug("Unable to determine IP from config (agent not available)")
	return []string{}, nil
}

func (pc *ProxmoxClient) parseNetworkConfig(netConfig string) []string {
	var ips []string
	
	// Look for IP addresses in network configuration
	// This is a simple regex-based approach
	ipRegex := regexp.MustCompile(`ip=([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3})`)
	matches := ipRegex.FindAllStringSubmatch(netConfig, -1)
	
	for _, match := range matches {
		if len(match) > 1 {
			ips = append(ips, match[1])
		}
	}
	
	return ips
}

func (pc *ProxmoxClient) applyMultiIPv4Strategy(ips []string) []string {
	if pc.config.ProxmoxMultiIPv4 == "all" {
		return ips
	}
	
	// Return first IPv4 and all IPv6 addresses
	var result []string
	var foundIPv4 bool
	
	for _, ip := range ips {
		if netIP := net.ParseIP(ip); netIP != nil {
			if netIP.To4() != nil {
				// IPv4 address
				if !foundIPv4 {
					result = append(result, ip)
					foundIPv4 = true
				}
			} else {
				// IPv6 address - always include
				result = append(result, ip)
			}
		}
	}
	
	return result
}