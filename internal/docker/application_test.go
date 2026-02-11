package docker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNameFromImageRef(t *testing.T) {
	assert.Equal(t, "once-campfire", NameFromImageRef("ghcr.io/basecamp/once-campfire:main"))
	assert.Equal(t, "once-campfire", NameFromImageRef("ghcr.io/basecamp/once-campfire"))
	assert.Equal(t, "nginx", NameFromImageRef("nginx:latest"))
	assert.Equal(t, "nginx", NameFromImageRef("nginx"))
}

func TestBuildEnvWithSMTP(t *testing.T) {
	settings := ApplicationSettings{
		SMTP: SMTPSettings{
			Server:   "smtp.example.com",
			Port:     "587",
			Username: "user@example.com",
			Password: "secret",
			From:     "noreply@example.com",
		},
	}

	env := settings.BuildEnv("test-secret-key")

	assert.Contains(t, env, "SMTP_ADDRESS=smtp.example.com")
	assert.Contains(t, env, "SMTP_PORT=587")
	assert.Contains(t, env, "SMTP_USERNAME=user@example.com")
	assert.Contains(t, env, "SMTP_PASSWORD=secret")
	assert.Contains(t, env, "MAILER_FROM_ADDRESS=noreply@example.com")
}

func TestBuildEnvWithoutSMTP(t *testing.T) {
	settings := ApplicationSettings{}

	env := settings.BuildEnv("test-secret-key")

	for _, e := range env {
		assert.NotContains(t, e, "SMTP_")
	}
}

func TestContainerResourcesEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 1, MemoryMB: 512}}

	differentCPUs := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 2, MemoryMB: 512}}
	assert.False(t, base.Equal(differentCPUs))

	differentMemory := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 1, MemoryMB: 1024}}
	assert.False(t, base.Equal(differentMemory))

	zeroResources := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(zeroResources))
}

func TestContainerResourcesMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:      "app",
		Image:     "img:latest",
		Resources: ContainerResources{CPUs: 2, MemoryMB: 512},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.Equal(t, 2, restored.Resources.CPUs)
	assert.Equal(t, 512, restored.Resources.MemoryMB)
	assert.True(t, original.Equal(restored))
}

func TestAutoUpdateEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", AutoUpdate: false}
	different := ApplicationSettings{Name: "app", AutoUpdate: true}
	assert.False(t, base.Equal(different))
}

func TestBackupSettingsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/backups", AutoBack: true}}

	differentPath := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/other", AutoBack: true}}
	assert.False(t, base.Equal(differentPath))

	differentAutoBack := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/backups", AutoBack: false}}
	assert.False(t, base.Equal(differentAutoBack))

	noBackup := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(noBackup))
}

func TestAutoUpdateAndBackupMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:       "app",
		Image:      "img:latest",
		AutoUpdate: true,
		Backup:     BackupSettings{Path: "/backups", AutoBack: true},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.True(t, restored.AutoUpdate)
	assert.Equal(t, "/backups", restored.Backup.Path)
	assert.True(t, restored.Backup.AutoBack)
	assert.True(t, original.Equal(restored))
}

func TestVerifyHTTP_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.VerifyHTTP(context.Background())
	assert.NoError(t, err)
}

func TestVerifyHTTP_RedirectToSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/home", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.VerifyHTTP(context.Background())
	assert.NoError(t, err)
}

func TestVerifyHTTP_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.VerifyHTTP(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerificationFailed)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestVerifyHTTP_Unreachable(t *testing.T) {
	app := &Application{
		Settings: ApplicationSettings{Host: "127.0.0.1:1", DisableTLS: true},
	}

	err := app.VerifyHTTP(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrVerificationFailed))
}

func TestVerifyHTTP_NoHost(t *testing.T) {
	app := &Application{
		Settings: ApplicationSettings{},
	}

	err := app.VerifyHTTP(context.Background())
	assert.NoError(t, err)
}
