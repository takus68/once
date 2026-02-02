package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
)

var (
	ErrApplicationExists = errors.New("application already exists")
	ErrInvalidBackup     = errors.New("invalid backup archive")
)

const BackupDataDir = "data"

type SMTPSettings struct {
	Server   string `json:"s,omitempty"`
	Port     string `json:"p,omitempty"`
	Username string `json:"u,omitempty"`
	Password string `json:"pw,omitempty"`
	From     string `json:"f,omitempty"`
}

func (s SMTPSettings) Equal(other SMTPSettings) bool {
	return s.Server == other.Server && s.Port == other.Port && s.Username == other.Username && s.Password == other.Password && s.From == other.From
}

func (s SMTPSettings) BuildEnv() []string {
	if s.Server == "" {
		return nil
	}
	return []string{
		"SMTP_ADDRESS=" + s.Server,
		"SMTP_PORT=" + s.Port,
		"SMTP_USERNAME=" + s.Username,
		"SMTP_PASSWORD=" + s.Password,
		"MAILER_FROM_ADDRESS=" + s.From,
	}
}

type ApplicationSettings struct {
	Name       string            `json:"n"`
	Image      string            `json:"i"`
	Host       string            `json:"h"`
	DisableTLS bool              `json:"dt"`
	EnvVars    map[string]string `json:"env"`
	SMTP       SMTPSettings      `json:"sm"`
}

func UnmarshalApplicationSettings(s string) (ApplicationSettings, error) {
	var settings ApplicationSettings
	err := json.Unmarshal([]byte(s), &settings)
	return settings, err
}

func (s ApplicationSettings) Marshal() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (s ApplicationSettings) TLSEnabled() bool {
	return s.Host != "" && !s.DisableTLS && !IsLocalhost(s.Host)
}

func (s ApplicationSettings) URL() string {
	if s.Host == "" {
		return ""
	}
	if s.TLSEnabled() {
		return "https://" + s.Host
	}
	return "http://" + s.Host
}

func (s ApplicationSettings) Equal(other ApplicationSettings) bool {
	if s.Name != other.Name || s.Image != other.Image || s.Host != other.Host || s.DisableTLS != other.DisableTLS {
		return false
	}
	if !s.SMTP.Equal(other.SMTP) {
		return false
	}
	if len(s.EnvVars) != len(other.EnvVars) {
		return false
	}
	for k, v := range s.EnvVars {
		if other.EnvVars[k] != v {
			return false
		}
	}
	return true
}

func (s ApplicationSettings) BuildEnv(secretKeyBase string) []string {
	env := []string{
		"SECRET_KEY_BASE=" + secretKeyBase,
	}

	if !s.TLSEnabled() {
		env = append(env, "DISABLE_SSL=true")
	}

	env = append(env, s.SMTP.BuildEnv()...)

	for k, v := range s.EnvVars {
		env = append(env, k+"="+v)
	}

	return env
}

type Application struct {
	namespace    *Namespace
	Settings     ApplicationSettings
	Running      bool
	RunningSince time.Time
}

func NewApplication(ns *Namespace, settings ApplicationSettings) *Application {
	return &Application{
		namespace: ns,
		Settings:  settings,
	}
}

func (a *Application) ContainerName(ctx context.Context) (string, error) {
	prefix := fmt.Sprintf("%s-app-%s-", a.namespace.name, a.Settings.Name)

	containers, err := a.namespace.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", err
	}

	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if strings.HasPrefix(name, prefix) {
			return name, nil
		}
	}

	return "", fmt.Errorf("no container found for app %s", a.Settings.Name)
}

func (a *Application) Volume(ctx context.Context) (*ApplicationVolume, error) {
	return FindOrCreateVolume(ctx, a.namespace, a.Settings.Name)
}

func (a *Application) Stop(ctx context.Context) error {
	name, err := a.ContainerName(ctx)
	if err != nil {
		return err
	}

	return a.namespace.client.ContainerStop(ctx, name, container.StopOptions{})
}

func (a *Application) Start(ctx context.Context) error {
	name, err := a.ContainerName(ctx)
	if err != nil {
		return err
	}

	return a.namespace.client.ContainerStart(ctx, name, container.StartOptions{})
}

func (a *Application) Update(ctx context.Context, progress DeployProgressCallback) error {
	return a.Deploy(ctx, progress)
}

func (a *Application) Deploy(ctx context.Context, progress DeployProgressCallback) error {
	reader, err := a.namespace.client.ImagePull(ctx, a.Settings.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image: %w", err)
	}
	defer reader.Close()

	if progress != nil {
		tracker := newPullProgressTracker(progress)
		if err := tracker.Track(reader); err != nil {
			return fmt.Errorf("tracking pull progress: %w", err)
		}
		progress(DeployProgress{Stage: DeployStageStarting})
	} else {
		_, _ = io.Copy(io.Discard, reader)
	}

	vol, err := a.Volume(ctx)
	if err != nil {
		return fmt.Errorf("getting volume: %w", err)
	}

	id, err := ContainerRandomID()
	if err != nil {
		return fmt.Errorf("generating container id: %w", err)
	}

	containerName := fmt.Sprintf("%s-app-%s-%s", a.namespace.name, a.Settings.Name, id)

	env := a.Settings.BuildEnv(vol.SecretKeyBase())

	resp, err := a.namespace.client.ContainerCreate(ctx,
		a.containerConfig(env),
		&container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyAlways},
			LogConfig:     ContainerLogConfig(),
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: vol.Name(),
					Target: "/rails/storage",
				},
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				a.namespace.name: {},
			},
		},
		nil,
		containerName,
	)
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	if err := a.namespace.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	shortContainerID := resp.ID[:12]

	if err := a.namespace.Proxy().Deploy(ctx, DeployOptions{
		AppName: a.Settings.Name,
		Target:  shortContainerID,
		Host:    a.Settings.Host,
		TLS:     a.Settings.TLSEnabled(),
	}); err != nil {
		a.namespace.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return fmt.Errorf("registering with proxy: %w", err)
	}

	if err := a.removeContainersExcept(ctx, containerName); err != nil {
		return fmt.Errorf("removing old containers: %w", err)
	}

	if progress != nil {
		progress(DeployProgress{Stage: DeployStageFinished})
	}

	return nil
}

func (a *Application) Backup(ctx context.Context, w io.Writer) error {
	containerName, err := a.ContainerName(ctx)
	if err != nil {
		return fmt.Errorf("getting container name: %w", err)
	}

	if err := a.runHookScript(ctx, containerName, "pre-backup"); err != nil {
		return fmt.Errorf("running pre-backup script: %w", err)
	}

	vol, err := a.Volume(ctx)
	if err != nil {
		return fmt.Errorf("getting volume: %w", err)
	}

	gw := gzip.NewWriter(w)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	if err := writeTarEntry(tw, "amar.application.json", []byte(a.Settings.Marshal())); err != nil {
		return fmt.Errorf("writing application settings: %w", err)
	}

	if err := writeTarEntry(tw, "amar.volume.json", []byte(vol.Settings.Marshal())); err != nil {
		return fmt.Errorf("writing volume settings: %w", err)
	}

	reader, _, err := a.namespace.client.CopyFromContainer(ctx, containerName, "/rails/storage")
	if err != nil {
		return fmt.Errorf("copying from container: %w", err)
	}
	defer reader.Close()

	if err := copyTarEntriesWithPrefix(reader, tw, "storage", BackupDataDir); err != nil {
		return fmt.Errorf("copying volume contents: %w", err)
	}

	return nil
}

func (a *Application) Destroy(ctx context.Context, destroyVolumes bool) error {
	prefix := fmt.Sprintf("%s-app-%s-", a.namespace.name, a.Settings.Name)

	containers, err := a.namespace.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers {
		for _, name := range c.Names {
			name = strings.TrimPrefix(name, "/")
			if strings.HasPrefix(name, prefix) {
				if err := a.namespace.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
					return fmt.Errorf("removing container: %w", err)
				}
				break
			}
		}
	}

	if destroyVolumes {
		vol, err := a.Volume(ctx)
		if err != nil {
			return fmt.Errorf("getting volume: %w", err)
		}
		if err := vol.Destroy(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Private

func (a *Application) runHookScript(ctx context.Context, containerName, name string) error {
	cmd := []string{"/scripts/" + name}

	execResp, err := a.namespace.client.ContainerExecCreate(ctx, containerName, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("creating exec: %w", err)
	}

	resp, err := a.namespace.client.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("attaching exec: %w", err)
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, resp.Reader); err != nil {
		return fmt.Errorf("reading exec output: %w", err)
	}

	inspect, err := a.namespace.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return fmt.Errorf("inspecting exec: %w", err)
	}

	// Exit codes 126 (not executable) and 127 (not found) mean the script doesn't exist
	if inspect.ExitCode == 126 || inspect.ExitCode == 127 {
		return nil
	}

	if inspect.ExitCode != 0 {
		return fmt.Errorf("hook script %q failed with exit code %d: %s", name, inspect.ExitCode, stderr.String())
	}

	return nil
}

func (a *Application) containerConfig(env []string) *container.Config {
	return &container.Config{
		Image: a.Settings.Image,
		Labels: map[string]string{
			"amar": a.Settings.Marshal(),
		},
		Env: env,
	}
}

func (a *Application) removeContainersExcept(ctx context.Context, keep string) error {
	prefix := fmt.Sprintf("%s-app-%s-", a.namespace.name, a.Settings.Name)

	containers, err := a.namespace.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if strings.HasPrefix(name, prefix) && name != keep {
			if err := a.namespace.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
				return err
			}
		}
	}

	return nil
}

// Helpers

func IsLocalhost(host string) bool {
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

func NameFromImageRef(imageRef string) string {
	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return imageRef
	}
	path := reference.Path(named)
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func ContainerRandomID() (string, error) {
	return randomID(6)
}

func randomID(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func copyTarEntriesWithPrefix(src io.Reader, dst *tar.Writer, oldPrefix, newPrefix string) error {
	tr := tar.NewReader(src)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if oldPrefix != "" && newPrefix != "" {
			if header.Name == oldPrefix {
				header.Name = newPrefix
			} else if strings.HasPrefix(header.Name, oldPrefix+"/") {
				header.Name = newPrefix + strings.TrimPrefix(header.Name, oldPrefix)
			}
		}

		if err := dst.WriteHeader(header); err != nil {
			return err
		}

		if header.Size > 0 {
			if _, err := io.Copy(dst, tr); err != nil {
				return err
			}
		}
	}
}
