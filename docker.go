package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
)

type DockerClient struct {
	client    *client.Client
	etcdClient *EtcdClient
}

func NewDockerClient(etcdClient *EtcdClient) (*DockerClient, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &DockerClient{
		client:     dockerClient,
		etcdClient: etcdClient,
	}, nil
}

func (dc *DockerClient) extractHostsFromLabels(labels map[string]string) []string {
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

func (dc *DockerClient) handleContainerEvent(event events.Message) {
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
	
	container, err := dc.client.ContainerInspect(ctx, containerID)
	if err != nil {
		log.WithFields(map[string]interface{}{
			"container_id": containerID,
			"error":        err,
		}).Error("Failed to inspect container")
		return
	}
	
	hosts := dc.extractHostsFromLabels(container.Config.Labels)
	if len(hosts) == 0 {
		return
	}
	
	// Create DNS records for all hosts
	log.WithFields(map[string]interface{}{
		"container_id":   containerID,
		"container_name": container.Name,
		"hosts":          hosts,
	}).Info("Processing Docker container for DNS records")
	
	for _, host := range hosts {
		if err := dc.etcdClient.CreateDNSRecord(host); err != nil {
			log.WithFields(map[string]interface{}{
				"host":  host,
				"error": err,
			}).Error("Failed to create DNS record")
		}
	}
}

func (dc *DockerClient) SyncExistingContainers() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	containers, err := dc.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	
	log.WithField("container_count", len(containers)).Info("Syncing existing containers")
	
	for _, container := range containers {
		hosts := dc.extractHostsFromLabels(container.Labels)
		if len(hosts) > 0 {
			log.WithFields(map[string]interface{}{
				"container_id":   container.ID,
				"container_name": strings.Join(container.Names, ","),
				"hosts":          hosts,
			}).Debug("Found hosts in container labels")
			
			for _, host := range hosts {
				if err := dc.etcdClient.CreateDNSRecord(host); err != nil {
					log.WithFields(map[string]interface{}{
						"host":  host,
						"error": err,
					}).Error("Failed to create DNS record during sync")
				}
			}
		}
	}
	
	return nil
}

func (dc *DockerClient) StartEventMonitoring(ctx context.Context) error {
	log.Info("Starting Docker event monitoring...")
	
	if err := dc.SyncExistingContainers(); err != nil {
		log.WithError(err).Warn("Failed to sync existing containers")
	}
	
	eventChan, errChan := dc.client.Events(ctx, events.ListOptions{})
	
	log.Info("Listening for Docker events...")
	
	for {
		select {
		case event := <-eventChan:
			dc.handleContainerEvent(event)
		case err := <-errChan:
			if err != nil {
				log.WithError(err).Error("Docker events stream error")
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (dc *DockerClient) Close() {
	if dc.client != nil {
		dc.client.Close()
	}
}