package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/systemd"
)

const unitTemplate = `[Unit]
Description=Once background tasks (%s)
After=network.target docker.service

[Service]
Type=simple
ExecStart=%s background run --namespace %s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

type BackgroundInstallCommand struct {
	root *RootCommand
	cmd  *cobra.Command
}

func NewBackgroundInstallCommand(root *RootCommand) *BackgroundInstallCommand {
	b := &BackgroundInstallCommand{root: root}
	b.cmd = &cobra.Command{
		Use:   "install",
		Short: "Install background tasks as a systemd service",
		Args:  cobra.NoArgs,
		RunE:  b.run,
	}
	return b
}

func (b *BackgroundInstallCommand) Command() *cobra.Command {
	return b.cmd
}

// Private

func (b *BackgroundInstallCommand) run(cmd *cobra.Command, args []string) error {
	if os.Getuid() != 0 {
		return fmt.Errorf("must be run as root")
	}

	ctx := context.Background()

	namespace, _ := cmd.Root().PersistentFlags().GetString("namespace")

	execPath, err := executablePath()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	serviceName := namespace + "-background"
	unitContent := fmt.Sprintf(unitTemplate, namespace, execPath, namespace)

	if systemd.IsInstalled(serviceName) {
		fmt.Printf("Service %s.service is already installed\n", serviceName)
		return nil
	}

	if err := systemd.Install(ctx, serviceName, unitContent); err != nil {
		return fmt.Errorf("installing service: %w", err)
	}

	fmt.Printf("Installed and started %s.service\n", serviceName)
	return nil
}

// Helpers

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
