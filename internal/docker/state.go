package docker

import "time"

type State struct {
	Apps map[string]*AppState `json:"apps"`
}

type AppState struct {
	LastBackup *OperationResult `json:"lb,omitempty"`
	LastUpdate *OperationResult `json:"lu,omitempty"`
}

func (as *AppState) LastBackupResult() *OperationResult {
	if as == nil {
		return nil
	}
	return as.LastBackup
}

func (as *AppState) LastUpdateResult() *OperationResult {
	if as == nil {
		return nil
	}
	return as.LastUpdate
}

type OperationResult struct {
	At    time.Time `json:"at"`
	Error string    `json:"err,omitempty"`
}

func (s *State) BackupDue(appName string) bool {
	return s.operationDue(appName, func(as *AppState) *OperationResult { return as.LastBackup })
}

func (s *State) UpdateDue(appName string) bool {
	return s.operationDue(appName, func(as *AppState) *OperationResult { return as.LastUpdate })
}

func (s *State) AppState(appName string) *AppState {
	if s.Apps == nil {
		return nil
	}
	return s.Apps[appName]
}

func (s *State) RecordBackup(appName string, err error) {
	s.ensureApp(appName).LastBackup = newOperationResult(err)
}

func (s *State) RecordUpdate(appName string, err error) {
	s.ensureApp(appName).LastUpdate = newOperationResult(err)
}

// Private

func (s *State) operationDue(appName string, getResult func(*AppState) *OperationResult) bool {
	app, ok := s.Apps[appName]
	if !ok {
		return true
	}

	result := getResult(app)
	if result == nil {
		return true
	}

	return result.Error != "" || time.Since(result.At) >= AutomaticTaskInterval
}

func (s *State) ensureApp(appName string) *AppState {
	if s.Apps == nil {
		s.Apps = make(map[string]*AppState)
	}

	app, ok := s.Apps[appName]
	if !ok {
		app = &AppState{}
		s.Apps[appName] = app
	}

	return app
}

// Helpers

func newOperationResult(err error) *OperationResult {
	r := &OperationResult{At: time.Now()}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}
