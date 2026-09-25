package device

import "errors"

// Stages runs named steps and reports each as running, then ok or fail.
type Stages struct{ R Reporter }

// Do runs fn as stage name. A failure is returned as a *StageError for that
// stage, carrying the failed command when fn returned a StageError.
func (s Stages) Do(name string, fn func() (detail string, err error)) error {
	s.R.Report(Stage{Name: name, Status: StageRunning})
	detail, err := fn()
	if err != nil {
		var se *StageError
		if !errors.As(err, &se) {
			se = &StageError{Err: err}
		}
		se.Stage = name
		detail := err.Error()
		if se.Err != nil {
			detail = se.Err.Error()
		}
		s.R.Report(Stage{Name: name, Status: StageFail, Detail: detail, Command: se.Command})
		return se
	}
	s.R.Report(Stage{Name: name, Status: StageOK, Detail: detail})
	return nil
}

// Warn reports a stage that completed with a caveat.
func (s Stages) Warn(name, detail string) {
	s.R.Report(Stage{Name: name, Status: StageWarn, Detail: detail})
}

// Skip reports a stage that did not need to run.
func (s Stages) Skip(name, detail string) {
	s.R.Report(Stage{Name: name, Status: StageSkip, Detail: detail})
}
