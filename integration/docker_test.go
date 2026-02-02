package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/amar/internal/docker"
)

func TestDockerDeployment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("amar-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "campfire",
		Image: "ghcr.io/basecamp/once-campfire:main",
	})
	require.NoError(t, app.Deploy(ctx, nil))
}

func TestRestoreState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns1, err := docker.NewNamespace("amar-restore-test")
	require.NoError(t, err)
	defer ns1.Teardown(ctx, true)

	require.NoError(t, ns1.EnsureNetwork(ctx))

	proxySettings := getProxyPorts(t)
	require.NoError(t, ns1.Proxy().Boot(ctx, proxySettings))

	app := ns1.AddApplication(docker.ApplicationSettings{
		Name:  "testapp",
		Image: "ghcr.io/basecamp/once-campfire:main",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	ns2, err := docker.RestoreNamespace(ctx, "amar-restore-test")
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

	ns1, err := docker.NewNamespace("amar-volume-test")
	require.NoError(t, err)

	require.NoError(t, ns1.EnsureNetwork(ctx))
	require.NoError(t, ns1.Proxy().Boot(ctx, getProxyPorts(t)))

	testFile := "/home/kamal-proxy/.config/kamal-proxy/test-persistence.txt"
	require.NoError(t, ns1.Proxy().Exec(ctx, []string{"sh", "-c", "echo 'hello' > " + testFile}))
	require.NoError(t, ns1.Teardown(ctx, false))

	ns2, err := docker.NewNamespace("amar-volume-test")
	require.NoError(t, err)
	defer ns2.Teardown(ctx, true)

	require.NoError(t, ns2.EnsureNetwork(ctx))
	require.NoError(t, ns2.Proxy().Boot(ctx, getProxyPorts(t)))
	require.NoError(t, ns2.Proxy().Exec(ctx, []string{"test", "-f", testFile}), "test file should exist after reboot")
}

func TestApplicationVolume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("amar-volume-label-test")
	require.NoError(t, err)

	vol1, err := docker.FindOrCreateVolume(ctx, ns, "testapp")
	require.NoError(t, err)
	assert.NotEmpty(t, vol1.SecretKeyBase())

	vol2, err := docker.FindOrCreateVolume(ctx, ns, "testapp")
	require.NoError(t, err)
	assert.Equal(t, vol1.SecretKeyBase(), vol2.SecretKeyBase())

	require.NoError(t, vol1.Destroy(ctx))
}

func TestGaplessDeployment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("amar-gapless-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "gapless",
		Image: "ghcr.io/basecamp/once-campfire:main",
	})

	require.NoError(t, app.Deploy(ctx, nil), "first deploy")

	vol, err := app.Volume(ctx)
	require.NoError(t, err)
	firstSecretKeyBase := vol.SecretKeyBase()

	firstName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	containerPrefix := "amar-gapless-test-app-gapless-"
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

	ns, err := docker.NewNamespace("amar-large-label-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "largelabel",
		Image: "ghcr.io/basecamp/once-campfire:main",
		EnvVars: map[string]string{
			"LARGE_VALUE": largeValue,
		},
	})
	require.NoError(t, app.Deploy(ctx, nil))

	ns2, err := docker.RestoreNamespace(ctx, "amar-large-label-test")
	require.NoError(t, err)

	restoredApp := ns2.Application("largelabel")
	require.NotNil(t, restoredApp)
	assert.Equal(t, largeValue, restoredApp.Settings.EnvVars["LARGE_VALUE"])
}

func TestStartStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("amar-startstop-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "startstop",
		Image: "ghcr.io/basecamp/once-campfire:main",
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

	ns, err := docker.NewNamespace("amar-long-name-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  longName,
		Image: "ghcr.io/basecamp/once-campfire:main",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	ns2, err := docker.RestoreNamespace(ctx, "amar-long-name-test")
	require.NoError(t, err)

	restoredApp := ns2.Application(longName)
	require.NotNil(t, restoredApp)
	assert.Equal(t, longName, restoredApp.Settings.Name)
}

func TestContainerLogConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("amar-logconfig-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "logtest",
		Image: "ghcr.io/basecamp/once-campfire:main",
	})
	require.NoError(t, app.Deploy(ctx, nil))

	assertContainerLogConfig(t, ctx, "amar-logconfig-test-proxy")

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)
	assertContainerLogConfig(t, ctx, containerName)
}

func TestBackup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("amar-backup-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := "ghcr.io/basecamp/once-campfire:main"
	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "backupapp",
		Image: imageName,
	})
	require.NoError(t, app.Deploy(ctx, nil))

	containerName, err := app.ContainerName(ctx)
	require.NoError(t, err)

	// Create a test file in storage
	execInContainer(t, ctx, containerName, []string{
		"sh", "-c", "echo 'test content' > /rails/storage/testfile.txt",
	})

	var buf bytes.Buffer
	require.NoError(t, app.Backup(ctx, &buf))

	entries := extractTarGz(t, &buf)

	assert.Contains(t, entries, "amar.application.json")
	var appSettings docker.ApplicationSettings
	require.NoError(t, json.Unmarshal(entries["amar.application.json"], &appSettings))
	assert.Equal(t, "backupapp", appSettings.Name)
	assert.Equal(t, imageName, appSettings.Image)

	assert.Contains(t, entries, "amar.volume.json")
	var volSettings docker.ApplicationVolumeSettings
	require.NoError(t, json.Unmarshal(entries["amar.volume.json"], &volSettings))
	assert.NotEmpty(t, volSettings.SecretKeyBase)

	assert.Contains(t, entries, "data/testfile.txt")
	assert.Equal(t, "test content\n", string(entries["data/testfile.txt"]))
}

func TestRestore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create and backup an app
	ns1, err := docker.NewNamespace("amar-restore-src")
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

	var backupBuf bytes.Buffer
	require.NoError(t, app.Backup(ctx, &backupBuf))

	// Restore to a different namespace
	ns2, err := docker.NewNamespace("amar-restore-dst")
	require.NoError(t, err)
	defer ns2.Teardown(ctx, true)

	require.NoError(t, ns2.EnsureNetwork(ctx))
	require.NoError(t, ns2.Proxy().Boot(ctx, getProxyPorts(t)))

	restoredApp, err := ns2.Restore(ctx, &backupBuf)
	require.NoError(t, err)

	// Verify settings were restored
	assert.Equal(t, "restoreapp", restoredApp.Settings.Name)
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
	ns3, err := docker.RestoreNamespace(ctx, "amar-restore-dst")
	require.NoError(t, err)

	restoredAppFromState := ns3.Application("restoreapp")
	require.NotNil(t, restoredAppFromState, "app should be discoverable after restore")
	assert.Equal(t, imageName, restoredAppFromState.Settings.Image)
	assert.Equal(t, "restore.localhost", restoredAppFromState.Settings.Host)

	volFromState, err := restoredAppFromState.Volume(ctx)
	require.NoError(t, err)
	assert.Equal(t, originalSecretKeyBase, volFromState.SecretKeyBase(), "volume SecretKeyBase should be preserved")
}

func TestRestoreExistingAppFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := docker.NewNamespace("amar-restore-exists-test")
	require.NoError(t, err)
	defer ns.Teardown(ctx, true)

	require.NoError(t, ns.EnsureNetwork(ctx))
	require.NoError(t, ns.Proxy().Boot(ctx, getProxyPorts(t)))

	imageName := "ghcr.io/basecamp/once-campfire:main"
	app := ns.AddApplication(docker.ApplicationSettings{
		Name:  "existingapp",
		Image: imageName,
	})
	require.NoError(t, app.Deploy(ctx, nil))

	var backupBuf bytes.Buffer
	require.NoError(t, app.Backup(ctx, &backupBuf))

	// Try to restore in the same namespace where the app already exists
	_, err = ns.Restore(ctx, &backupBuf)
	assert.ErrorIs(t, err, docker.ErrApplicationExists)
}

// Helpers

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
		if err == io.EOF {
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

