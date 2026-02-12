package background

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/basecamp/once/internal/docker"
)

const CheckInterval = 5 * time.Minute

type Runner struct {
	namespace string
	logger    *slog.Logger
}

func NewRunner(namespace string, logger *slog.Logger) *Runner {
	return &Runner{
		namespace: namespace,
		logger:    logger,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	r.logger.Info("Starting background runner", "namespace", r.namespace, "check_interval", CheckInterval)

	if err := r.check(ctx); err != nil {
		r.logger.Error("Check failed", "error", err)
	}

	ticker := time.NewTicker(CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Shutting down")
			return nil
		case <-ticker.C:
			if err := r.check(ctx); err != nil {
				r.logger.Error("Check failed", "error", err)
			}
		}
	}
}

// Private

func (r *Runner) check(ctx context.Context) error {
	ns, err := docker.RestoreNamespace(ctx, r.namespace)
	if err != nil {
		return fmt.Errorf("restoring namespace: %w", err)
	}

	state, err := ns.LoadState(ctx)
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	for _, app := range ns.Applications() {
		if !app.Running {
			continue
		}

		r.checkUpdate(ctx, app, state)
		r.checkBackup(ctx, app, state)
	}

	return nil
}

func (r *Runner) checkUpdate(ctx context.Context, app *docker.Application, state *docker.State) {
	if !app.Settings.AutoUpdate {
		return
	}
	if !state.UpdateDue(app.Settings.Name) {
		return
	}

	r.logger.Info("Running auto-update", "app", app.Settings.Name)

	changed, err := app.Update(ctx, nil)
	if err != nil {
		r.logger.Error("Auto-update failed", "app", app.Settings.Name, "error", err)
	} else if changed {
		r.logger.Info("Auto-update completed", "app", app.Settings.Name)
	} else {
		r.logger.Info("Already up to date", "app", app.Settings.Name)
	}
}

func (r *Runner) checkBackup(ctx context.Context, app *docker.Application, state *docker.State) {
	if !app.Settings.Backup.AutoBack {
		return
	}
	if !state.BackupDue(app.Settings.Name) {
		return
	}

	r.logger.Info("Running auto-backup", "app", app.Settings.Name)

	if err := app.Backup(ctx); err != nil {
		r.logger.Error("Auto-backup failed", "app", app.Settings.Name, "error", err)
	} else {
		r.logger.Info("Auto-backup completed", "app", app.Settings.Name)
	}

	if err := app.TrimBackups(); err != nil {
		r.logger.Error("Backup trim failed", "app", app.Settings.Name, "error", err)
	}
}
