package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const DefaultNamespace = "amar"

type Namespace struct {
	name         string
	client       *client.Client
	proxy        *Proxy
	applications []*Application
}

func NewNamespace(name string) (*Namespace, error) {
	if name == "" {
		name = DefaultNamespace
	}

	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	ns := &Namespace{
		name:   name,
		client: c,
	}
	ns.proxy = NewProxy(ns)
	return ns, nil
}

func RestoreNamespace(ctx context.Context, name string) (*Namespace, error) {
	ns, err := NewNamespace(name)
	if err != nil {
		return nil, err
	}

	if err := ns.restoreState(ctx); err != nil {
		return nil, err
	}

	return ns, nil
}

func (n *Namespace) AddApplication(settings ApplicationSettings) *Application {
	app := NewApplication(n, settings)
	n.applications = append(n.applications, app)
	n.sortApplications()
	return app
}

func (n *Namespace) Proxy() *Proxy {
	return n.proxy
}

func (n *Namespace) Application(name string) *Application {
	for _, app := range n.applications {
		if app.Settings.Name == name {
			return app
		}
	}
	return nil
}

func (n *Namespace) Applications() []*Application {
	return n.applications
}

func (n *Namespace) Setup(ctx context.Context) error {
	if err := n.EnsureNetwork(ctx); err != nil {
		return err
	}

	if n.proxy.Settings == nil {
		if err := n.proxy.Boot(ctx, ProxySettings{}); err != nil {
			return err
		}
	}

	return nil
}

func (n *Namespace) EnsureNetwork(ctx context.Context) error {
	networks, err := n.client.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return err
	}

	for _, net := range networks {
		if net.Name == n.name {
			return nil
		}
	}

	_, err = n.client.NetworkCreate(ctx, n.name, network.CreateOptions{
		Driver: "bridge",
	})
	return err
}

func (n *Namespace) Teardown(ctx context.Context, destroyVolumes bool) error {
	for _, app := range n.applications {
		if err := app.Destroy(ctx, destroyVolumes); err != nil {
			return err
		}
	}

	if err := n.proxy.Destroy(ctx, destroyVolumes); err != nil {
		return err
	}

	return n.client.NetworkRemove(ctx, n.name)
}

func (n *Namespace) Refresh(ctx context.Context) error {
	n.applications = nil
	return n.restoreState(ctx)
}

func (n *Namespace) EventWatcher() *EventWatcher {
	return NewEventWatcher(n.client, n.name)
}

func (n *Namespace) ApplicationExists(ctx context.Context, name string) (bool, error) {
	prefix := fmt.Sprintf("%s-app-%s-", n.name, name)

	containers, err := n.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return false, err
	}

	for _, c := range containers {
		for _, cname := range c.Names {
			cname = strings.TrimPrefix(cname, "/")
			if strings.HasPrefix(cname, prefix) {
				return true, nil
			}
		}
	}

	return false, nil
}

func (n *Namespace) Restore(ctx context.Context, r io.Reader) (*Application, error) {
	appSettings, volSettings, volumeData, err := n.parseBackup(r)
	if err != nil {
		return nil, fmt.Errorf("parsing backup: %w", err)
	}

	exists, err := n.ApplicationExists(ctx, appSettings.Name)
	if err != nil {
		return nil, fmt.Errorf("checking application exists: %w", err)
	}
	if exists {
		return nil, ErrApplicationExists
	}

	vol, err := CreateVolume(ctx, n, appSettings.Name, volSettings)
	if err != nil {
		return nil, fmt.Errorf("creating volume: %w", err)
	}

	if err := n.populateVolume(ctx, vol, appSettings.Image, volumeData); err != nil {
		vol.Destroy(ctx)
		return nil, fmt.Errorf("populating volume: %w", err)
	}

	app := n.AddApplication(appSettings)
	if err := app.Deploy(ctx, nil); err != nil {
		vol.Destroy(ctx)
		return nil, fmt.Errorf("deploying application: %w", err)
	}

	return app, nil
}

// Private

type appCandidate struct {
	app     *Application
	created int64
}

func (n *Namespace) restoreState(ctx context.Context) error {
	containers, err := n.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	proxyPrefix := n.name + "-proxy"
	appPrefix := n.name + "-app-"

	// Use a map to deduplicate apps by name, preferring the most recently created container
	appsByName := make(map[string]appCandidate)

	for _, c := range containers {
		for _, name := range c.Names {
			name = strings.TrimPrefix(name, "/")

			if name == proxyPrefix {
				label := c.Labels["amar"]
				if label != "" {
					settings, err := UnmarshalProxySettings(label)
					if err != nil {
						return err
					}
					n.proxy.Settings = &settings
				}
				break
			}

			if strings.HasPrefix(name, appPrefix) {
				label := c.Labels["amar"]
				if label != "" {
					settings, err := UnmarshalApplicationSettings(label)
					if err != nil {
						return err
					}
					app := NewApplication(n, settings)
					app.Running = c.State == "running"
					if app.Running {
						info, err := n.client.ContainerInspect(ctx, c.ID)
						if err == nil && info.State != nil {
							if t, err := time.Parse(time.RFC3339Nano, info.State.StartedAt); err == nil {
								app.RunningSince = t
							}
						}
					}

					existing, found := appsByName[settings.Name]
					if !found || c.Created > existing.created {
						appsByName[settings.Name] = appCandidate{app: app, created: c.Created}
					}
				}
				break
			}
		}
	}

	for _, candidate := range appsByName {
		n.applications = append(n.applications, candidate.app)
	}

	n.sortApplications()
	return nil
}

func (n *Namespace) sortApplications() {
	slices.SortFunc(n.applications, func(a, b *Application) int {
		return strings.Compare(a.Settings.Name, b.Settings.Name)
	})
}

func (n *Namespace) parseBackup(r io.Reader) (ApplicationSettings, ApplicationVolumeSettings, []byte, error) {
	var appSettings ApplicationSettings
	var volSettings ApplicationVolumeSettings
	var volumeData bytes.Buffer

	gr, err := gzip.NewReader(r)
	if err != nil {
		return appSettings, volSettings, nil, fmt.Errorf("%w: %v", ErrInvalidBackup, err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	tw := tar.NewWriter(&volumeData)
	defer tw.Close()

	foundApp := false
	foundVol := false

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return appSettings, volSettings, nil, fmt.Errorf("%w: %v", ErrInvalidBackup, err)
		}

		switch header.Name {
		case "amar.application.json":
			data, err := io.ReadAll(tr)
			if err != nil {
				return appSettings, volSettings, nil, fmt.Errorf("%w: reading application settings: %v", ErrInvalidBackup, err)
			}
			appSettings, err = UnmarshalApplicationSettings(string(data))
			if err != nil {
				return appSettings, volSettings, nil, fmt.Errorf("%w: parsing application settings: %v", ErrInvalidBackup, err)
			}
			foundApp = true

		case "amar.volume.json":
			data, err := io.ReadAll(tr)
			if err != nil {
				return appSettings, volSettings, nil, fmt.Errorf("%w: reading volume settings: %v", ErrInvalidBackup, err)
			}
			volSettings, err = UnmarshalApplicationVolumeSettings(string(data))
			if err != nil {
				return appSettings, volSettings, nil, fmt.Errorf("%w: parsing volume settings: %v", ErrInvalidBackup, err)
			}
			foundVol = true

		default:
			if header.Name == BackupDataDir || strings.HasPrefix(header.Name, BackupDataDir+"/") {
				newHeader := *header
				if header.Name == BackupDataDir {
					newHeader.Name = "data"
				} else {
					newHeader.Name = "data" + strings.TrimPrefix(header.Name, BackupDataDir)
				}
				if err := tw.WriteHeader(&newHeader); err != nil {
					return appSettings, volSettings, nil, err
				}
				if header.Size > 0 {
					if _, err := io.Copy(tw, tr); err != nil {
						return appSettings, volSettings, nil, err
					}
				}
			}
		}
	}

	if !foundApp || !foundVol {
		return appSettings, volSettings, nil, fmt.Errorf("%w: missing required metadata files", ErrInvalidBackup)
	}

	return appSettings, volSettings, volumeData.Bytes(), nil
}

func (n *Namespace) populateVolume(ctx context.Context, vol *ApplicationVolume, imageName string, data []byte) error {
	reader, err := n.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)

	containerName := fmt.Sprintf("%s-restore-temp", n.name)

	resp, err := n.client.ContainerCreate(ctx,
		&container.Config{
			Image:      imageName,
			Entrypoint: []string{},
			Cmd:        []string{"sleep", "infinity"},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: vol.Name(),
					Target: "/data",
				},
			},
		},
		nil,
		nil,
		containerName,
	)
	if err != nil {
		return fmt.Errorf("creating temp container: %w", err)
	}

	defer n.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := n.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting temp container: %w", err)
	}

	if len(data) > 0 {
		if err := n.client.CopyToContainer(ctx, resp.ID, "/", bytes.NewReader(data), container.CopyToContainerOptions{}); err != nil {
			return fmt.Errorf("copying data to volume: %w", err)
		}
	}

	return nil
}
