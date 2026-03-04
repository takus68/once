package integration

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestDockerDeployment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "campfire",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "campfire.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))
}

func TestRestoreState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns1, err := docker.NewNamespace("once-restore-test")
	require.NoError(t, err)
	defer ns1.Teardown(ctx, true)

	require.NoError(t, ns1.EnsureNetwork(ctx))

	proxySettings := getProxyPorts(t)
	require.NoError(t, ns1.Proxy().Boot(ctx, proxySettings))

	app := ns1.AddApplication(docker.ApplicationSettings{
		Name:  "testapp",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "testapp.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	ns2, err := docker.RestoreNamespace(ctx, "once-restore-test")
	require.NoError(t, err)

	require.NotNil(t, ns2.Proxy().Settings)
	assert.Equal(t, proxySettings.HTTPPort, ns2.Proxy().Settings.HTTPPort)
	assert.Equal(t, proxySettings.HTTPSPort, ns2.Proxy().Settings.HTTPSPort)

	restoredApp := ns2.Application("testapp")
	require.NotNil(t, restoredApp)
	assert.Equal(t, app.Settings.Image, restoredApp.Settings.Image)
}

func TestVolumePersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns1, err := docker.NewNamespace("once-volume-test")
	require.NoError(t, err)

	require.NoError(t, ns1.EnsureNetwork(ctx))
	require.NoError(t, ns1.Proxy().Boot(ctx, getProxyPorts(t)))

	testFile := "/home/kamal-proxy/.config/kamal-proxy/test-persistence.txt"
	require.NoError(t, ns1.Proxy().Exec(ctx, []string{"sh", "-c", "echo 'hello' > " + testFile}))
	require.NoError(t, ns1.Teardown(ctx, false))

	ns2, err := docker.NewNamespace("once-volume-test")
	require.NoError(t, err)
	defer ns2.Teardown(ctx, true)

	require.NoError(t, ns2.EnsureNetwork(ctx))
	require.NoError(t, ns2.Proxy().Boot(ctx, getProxyPorts(t)))
	require.NoError(t, ns2.Proxy().Exec(ctx, []string{"test", "-f", testFile}), "test file should exist after reboot")
}

func TestApplicationVolume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-volume-label-test")
	require.NoError(t, err)

	vol1, err := docker.CreateVolume(ctx, ns, "testapp", docker.ApplicationVolumeSettings{SecretKeyBase: "test-secret"})
	require.NoError(t, err)
	assert.Equal(t, "test-secret", vol1.SecretKeyBase())

	vol2, err := docker.FindVolume(ctx, ns, "testapp")
	require.NoError(t, err)
	assert.Equal(t, vol1.SecretKeyBase(), vol2.SecretKeyBase())

	require.NoError(t, vol1.Destroy(ctx))
}

func TestGaplessDeployment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-gapless-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "gapless",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "gapless.localhost",
	})

	require.NoError(t, app.Deploy(ctx, nil), "first deploy")

	vol, err := app.Volume(ctx)
	require.NoError(t, err)
	firstSecretKeyBase := vol.SecretKeyBase()

	firstName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	containerPrefix := "once-gapless-test-app-gapless-"
	countBefore := countContainers(t, ctx, containerPrefix)

	require.NoError(t, app.Deploy(ctx, nil), "second deploy")

	countAfter := countContainers(t, ctx, containerPrefix)
	assert.Equal(t, countBefore, countAfter, "container count should not change")

	vol2, err := app.Volume(ctx)
	require.NoError(t, err)
	assert.Equal(t, firstSecretKeyBase, vol2.SecretKeyBase(), "SecretKeyBase should persist across deploys")

	secondName, err := app.ContainerName(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, firstName, secondName, "container name should change between deploys")

	require.NoError(t, ns.Refresh(ctx))
	assert.Len(t, ns.Applications(), 1, "should have exactly one application after redeploy and refresh")
}

func TestLargeLabelData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	largeValue := strings.Repeat("x", 64*1024) // 64KB

	ns, err := docker.NewNamespace("once-large-label-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "largelabel",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "largelabel.localhost",
		EnvVars: map[string]string{
			"LARGE_VALUE": largeValue,
		},
	})
	require.NoError(t, app.Deploy(ctx, nil))

	ns2, err := docker.RestoreNamespace(ctx, "once-large-label-test")
	require.NoError(t, err)

	restoredApp := ns2.Application("largelabel")
	require.NotNil(t, restoredApp)
	assert.Equal(t, largeValue, restoredApp.Settings.EnvVars["LARGE_VALUE"])
}

func TestStartStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-startstop-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "startstop",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "startstop.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	assertContainerRunning(t, ctx, containerName, true)

	require.NoError(t, app.Stop(ctx))
	assertContainerRunning(t, ctx, containerName, false)

	require.NoError(t, app.Start(ctx))
	assertContainerRunning(t, ctx, containerName, true)
}

func TestLongAppName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Container names can be very long since we use container IDs for proxy targeting.
	// This test verifies that long app names work correctly.
	longName := strings.Repeat("x", 200)

	ns, err := docker.NewNamespace("once-long-name-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  longName,
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "longname.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	ns2, err := docker.RestoreNamespace(ctx, "once-long-name-test")
	require.NoError(t, err)

	restoredApp := ns2.Application(longName)
	require.NotNil(t, restoredApp)
	assert.Equal(t, longName, restoredApp.Settings.Name)
}

func TestContainerLogConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-logconfig-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "logtest",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "logtest.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	assertContainerLogConfig(t, ctx, "once-logconfig-test-proxy")

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)
	assertContainerLogConfig(t, ctx, containerName)
}

func TestBackup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-backup-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := "ghcr.io/basecamp/once-campfire:main"
	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "backupapp",
		Image: imageName,
		Host:  "backupapp.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	// Create a test file in storage
	execInContainer(t, ctx, containerName, []string{
		"sh", "-c", "echo 'test content' > /rails/storage/testfile.txt",
	})

	backupDir := t.TempDir()
	require.NoError(t, app.BackupToFile(ctx, backupDir, "backup.tar.gz"))

	backupFile, err := os.Open(filepath.Join(backupDir, "backup.tar.gz"))
	require.NoError(t, err)
	defer backupFile.Close()

	entries := extractTarGz(t, backupFile)

	assert.Contains(t, entries, "once.application.json")
	var appSettings docker.ApplicationSettings
	require.NoError(t, json.Unmarshal(entries["once.application.json"], &appSettings))
	assert.Equal(t, "backupapp", appSettings.Name)
	assert.Equal(t, imageName, appSettings.Image)

	assert.Contains(t, entries, "once.volume.json")
	var volSettings docker.ApplicationVolumeSettings
	require.NoError(t, json.Unmarshal(entries["once.volume.json"], &volSettings))
	assert.NotEmpty(t, volSettings.SecretKeyBase)

	assert.Contains(t, entries, "data/testfile.txt")
	assert.Equal(t, "test content\n", string(entries["data/testfile.txt"]))
}

func TestRestore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create and backup an app
	ns1, err := docker.NewNamespace("once-restore-src")
	require.NoError(t, err)
	defer ns1.Teardown(ctx, true)

	require.NoError(t, ns1.EnsureNetwork(ctx))
	require.NoError(t, ns1.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := "ghcr.io/basecamp/once-campfire:main"
	app := ns1.AddApplication(docker.ApplicationSettings{
		Name:  "restoreapp",
		Image: imageName,
		Host:  "restore.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	execInContainer(t, ctx, containerName, []string{
		"sh", "-c", "echo 'restore test data' > /rails/storage/restore-test.txt",
	})

	vol, err := app.Volume(ctx)
	require.NoError(t, err)
	originalSecretKeyBase := vol.SecretKeyBase()

	backupDir := t.TempDir()
	require.NoError(t, app.BackupToFile(ctx, backupDir, "backup.tar.gz"))

	// Restore to a different namespace
	ns2, err := docker.NewNamespace("once-restore-dst")
	require.NoError(t, err)
	defer ns2.Teardown(ctx, true)

	require.NoError(t, ns2.EnsureNetwork(ctx))
	require.NoError(t, ns2.Proxy().Boot(ctx, getProxyPorts(t)))

	backupFile, err := os.Open(filepath.Join(backupDir, "backup.tar.gz"))
	require.NoError(t, err)
	defer backupFile.Close()

	restoredApp, err := ns2.Restore(ctx, backupFile)
	require.NoError(t, err)

	// Verify the restored app gets a fresh unique name based on the image
	assert.True(t, strings.HasPrefix(restoredApp.Settings.Name, "once-campfire."), "restored name should start with image base name")
	assert.NotEqual(t, "restoreapp", restoredApp.Settings.Name)
	assert.Equal(t, imageName, restoredApp.Settings.Image)
	assert.Equal(t, "restore.localhost", restoredApp.Settings.Host)

	// Verify volume settings (SecretKeyBase) were preserved
	restoredVol, err := restoredApp.Volume(ctx)
	require.NoError(t, err)
	assert.Equal(t, originalSecretKeyBase, restoredVol.SecretKeyBase())

	// Verify data was restored
	restoredContainerName, err := restoredApp.ContainerName(ctx)
	require.NoError(t, err)

	execInContainer(t, ctx, restoredContainerName, []string{
		"test", "-f", "/rails/storage/restore-test.txt",
	})

	// Verify that the app and volume are properly labelled by restoring the namespace
	restoredName := restoredApp.Settings.Name
	ns3, err := docker.RestoreNamespace(ctx, "once-restore-dst")
	require.NoError(t, err)

	restoredAppFromState := ns3.Application(restoredName)
	require.NotNil(t, restoredAppFromState, "app should be discoverable after restore")
	assert.Equal(t, imageName, restoredAppFromState.Settings.Image)
	assert.Equal(t, "restore.localhost", restoredAppFromState.Settings.Host)

	volFromState, err := restoredAppFromState.Volume(ctx)
	require.NoError(t, err)
	assert.Equal(t, originalSecretKeyBase, volFromState.SecretKeyBase(), "volume SecretKeyBase should be preserved")
}

func TestRestoreHostnameConflictFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-restore-host-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := "ghcr.io/basecamp/once-campfire:main"
	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "existingapp",
		Image: imageName,
		Host:  "existingapp.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	backupDir := t.TempDir()
	require.NoError(t, app.BackupToFile(ctx, backupDir, "backup.tar.gz"))

	// Try to restore when another app already uses the same hostname
	backupFile, err := os.Open(filepath.Join(backupDir, "backup.tar.gz"))
	require.NoError(t, err)
	defer backupFile.Close()

	_, err = ns.Restore(ctx, backupFile)
	assert.ErrorIs(t, err, docker.ErrHostnameInUse)
}

func TestRemoveApplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-remove-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "removeapp",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "removeapp.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	containerPrefix := "once-remove-test-app-removeapp-"
	assert.Equal(t, 1, countContainers(t, ctx, containerPrefix))

	require.NoError(t, app.Remove(ctx, false))

	assert.Equal(t, 0, countContainers(t, ctx, containerPrefix))

	_, err = docker.FindVolume(ctx, ns, "removeapp")
	assert.NoError(t, err, "volume should still exist")
}

func TestRemoveApplicationWithData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-removedata-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "removeapp",
		Image: "ghcr.io/basecamp/once-campfire:main",
		Host:  "removeapp.localhost",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	containerPrefix := "once-removedata-test-app-removeapp-"
	assert.Equal(t, 1, countContainers(t, ctx, containerPrefix))

	require.NoError(t, app.Remove(ctx, true))

	assert.Equal(t, 0, countContainers(t, ctx, containerPrefix))

	_, err = docker.FindVolume(ctx, ns, "removeapp")
	assert.ErrorIs(t, err, docker.ErrVolumeNotFound)
}

// Helpers

func TestContainerResources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("once-res-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:      "campfire",
		Image:     "ghcr.io/basecamp/once-campfire:main",
		Host:      "campfire.localhost",
		Resources: docker.ContainerResources{CPUs: 1, MemoryMB: 1024},
	})
	require.NoError(t, app.Deploy(ctx, nil))

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	assertContainerResources(t, ctx, containerName, 1e9, 1024*1024*1024)
}

func getFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func getProxyPorts(t *testing.T) docker.ProxySettings {
	t.Helper()
	return docker.ProxySettings{
		HTTPPort:    getFreePort(t),
		HTTPSPort:   getFreePort(t),
		MetricsPort: getFreePort(t),
	}
}

func assertContainerRunning(t *testing.T, ctx context.Context, name string, expectRunning bool) {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	info, err := c.ContainerInspect(ctx, name)
	require.NoError(t, err)

	if expectRunning {
		assert.True(t, info.State.Running, "container should be running")
	} else {
		assert.False(t, info.State.Running, "container should be stopped")
	}
}

func assertContainerResources(t *testing.T, ctx context.Context, name string, expectedCPUs, expectedMemory int64) {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	info, err := c.ContainerInspect(ctx, name)
	require.NoError(t, err)

	assert.Equal(t, expectedCPUs, info.HostConfig.NanoCPUs)
	assert.Equal(t, expectedMemory, info.HostConfig.Memory)
}

func assertContainerLogConfig(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	info, err := c.ContainerInspect(ctx, name)
	require.NoError(t, err)

	assert.Equal(t, "json-file", info.HostConfig.LogConfig.Type)
	assert.Equal(t, docker.ContainerLogMaxSize, info.HostConfig.LogConfig.Config["max-size"])
	assert.Equal(t, "1", info.HostConfig.LogConfig.Config["max-file"])
}

func countContainers(t *testing.T, ctx context.Context, prefix string) int {
	t.Helper()
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	containers, err := c.ContainerList(ctx, container.ListOptions{All: true})
	require.NoError(t, err)

	count := 0
	for _, ctr := range containers {
		if len(ctr.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(ctr.Names[0], "/")
		if strings.HasPrefix(name, prefix) {
			count++
		}
	}
	return count
}

func execInContainer(t *testing.T, ctx context.Context, containerName string, cmd []string) {
	t.Helper()

	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer c.Close()

	execResp, err := c.ContainerExecCreate(ctx, containerName, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	require.NoError(t, err)

	resp, err := c.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	require.NoError(t, err)
	defer resp.Close()

	_, err = io.Copy(io.Discard, resp.Reader)
	require.NoError(t, err)

	inspect, err := c.ContainerExecInspect(ctx, execResp.ID)
	require.NoError(t, err)
	require.Equal(t, 0, inspect.ExitCode, "exec command failed")
}

func extractTarGz(t *testing.T, r io.Reader) map[string][]byte {
	t.Helper()

	gr, err := gzip.NewReader(r)
	require.NoError(t, err)
	defer gr.Close()

	tr := tar.NewReader(gr)
	entries := make(map[string][]byte)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		if header.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			require.NoError(t, err)
			entries[header.Name] = data
		}
	}

	return entries
}
