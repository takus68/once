package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type updateCommand struct {
	cmd   *cobra.Command
	flags settingsFlags
	image string
}

func newUpdateCommand() *updateCommand {
	u := &updateCommand{}
	u.cmd = &cobra.Command{
		Use:   "update <host>",
		Short: "Update settings for a deployed application",
		Args:  cobra.ExactArgs(1),
		RunE:  WithNamespace(u.run),
	}

	u.flags.register(u.cmd)
	u.cmd.Flags().StringVar(&u.image, "image", "", "new image for the application")

	return u
}

// Private

func (u *updateCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	currentHost := args[0]

	app := ns.ApplicationByHost(currentHost)
	if app == nil {
		return fmt.Errorf("no application found at host %q", currentHost)
	}

	if err := ns.Setup(ctx); err != nil {
		return fmt.Errorf("%w: %w", docker.ErrSetupFailed, err)
	}

	settings, err := u.flags.applyChanges(cmd, app.Settings)
	if err != nil {
		return err
	}

	if cmd.Flags().Changed("image") {
		settings.Image = u.image
	}

	if settings.Host != app.Settings.Host {
		if ns.HostInUseByAnother(settings.Host, app.Settings.Name) {
			return docker.ErrHostnameInUse
		}
	}

	oldSettings := app.Settings
	app.Settings = settings

	progress := func(p docker.DeployProgress) {
		switch p.Stage {
		case docker.DeployStageDownloading:
			fmt.Printf("Downloading: %d%%\n", p.Percentage)
		case docker.DeployStageStarting:
			fmt.Println("Starting...")
		case docker.DeployStageFinished:
			fmt.Println("Finished")
		}
	}

	if err := app.Deploy(ctx, progress); err != nil {
		app.Settings = oldSettings
		return fmt.Errorf("%w: %w", docker.ErrDeployFailed, err)
	}

	fmt.Printf("Updated %s\n", app.Settings.Name)
	return nil
}
