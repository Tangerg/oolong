package base

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// SubmissionID identifies one validated attempt, independently of the worker
// carrying it out. A later attempt revokes settlement of an earlier one.
type SubmissionID struct{ identity *byte }

const (
	ActionCancel         scene.ActionID = "cancel"
	ActionReadSubmission scene.ActionID = "read-submission"
)

// SubmissionState distinguishes a pending attempt from its terminal outcome.
type SubmissionState uint8

const (
	SubmissionIdle SubmissionState = iota
	SubmissionPending
	SubmissionSucceeded
	SubmissionFailed
	SubmissionCancelled
)

// SubmissionOutcome settles one asynchronous attempt. Cancellation and failure do not imply successful submission.
type SubmissionOutcome struct {
	Err       error
	Cancelled bool
}

// Submission is a snapshot. Current is true only while the submitted field
// revisions still describe the form's draft at observation time.
type Submission struct {
	ID      SubmissionID
	State   SubmissionState
	Err     error
	Current bool
}

type fieldVersion struct {
	field    Field
	revision uint64
}

var (
	// ErrSubmissionAborted reports that the submit callback exited without accepting work.
	ErrSubmissionAborted = errors.New("base: submission callback did not return")
	// ErrSubmissionStale reports a request superseded by a newer submission or field relation.
	ErrSubmissionStale = errors.New("base: submission is no longer current")
	// ErrNotCancellable reports that no active attempt accepts a cancellation request.
	ErrNotCancellable = errors.New("base: submission cannot be cancelled")
)

// Submit validates every field, captures their revisions and starts one identified attempt. Started is not completion. It panics during presentation or observation.
func (f *Form) Submit() (scene.ActionResult[SubmissionID], error) {
	return f.SubmitWith(nil)
}

// SubmitWith collects and validates each field through collect before submitting.
// A successful collector must leave a current Validation receipt on that field;
// a collection error stops immediately. A nil collector validates all fields.
// Changing the field relation or starting another attempt supersedes this call.
func (f *Form) SubmitWith(collect func(Field) error) (scene.ActionResult[SubmissionID], error) {
	requireMutable(f.Element())
	id := SubmissionID{identity: new(byte)}
	superseded := f.attempt
	f.problem = nil
	f.cancelFeedback.clear()
	f.attempt = Submission{ID: id, State: SubmissionIdle}
	f.submitted = nil
	// Validation may exit without returning after the previous attempt was revoked.
	f.Element().Invalidate()
	f.revoke(superseded)
	if f.attempt.ID != id {
		return scene.ActionResult[SubmissionID]{}, ErrSubmissionStale
	}
	versions, validationErr := f.validateSubmission(id, collect)
	if validationErr != nil {
		if f.attempt.ID == id {
			f.Element().Invalidate()
		}
		return scene.ActionResult[SubmissionID]{}, validationErr
	}
	f.submitted = versions
	state, err := f.start(id)
	if state != scene.Started && state != scene.Applied && state != scene.Unchanged {
		err = errors.Join(err, errors.New("base: submission callback returned an invalid action state"))
		if f.attempt.ID == id {
			f.complete(SubmissionOutcome{Err: err})
		}
		return scene.ActionResult[SubmissionID]{}, err
	}
	result := scene.ActionResult[SubmissionID]{State: state, Value: id}
	if f.attempt.ID != id {
		return result, errors.Join(err, ErrSubmissionStale)
	}
	if state == scene.Started {
		f.attempt.State, f.attempt.Err = SubmissionPending, err
		f.Element().Invalidate()
	} else {
		f.complete(SubmissionOutcome{Err: err})
	}
	return result, err
}

// The callback decides admission. Only work it actually accepted may be settled
// later; an abnormal exit propagates after recording failure on its own attempt.
func (f *Form) start(id SubmissionID) (scene.ActionState, error) {
	if f.submit == nil {
		return scene.Applied, nil
	}
	returned := false
	defer func() {
		if !returned && f.attempt.ID == id {
			f.complete(SubmissionOutcome{Err: ErrSubmissionAborted})
		}
	}()
	state, err := f.submit(id)
	returned = true
	return state, err
}

func (f *Form) currentValidations(validations []func() bool) bool {
	return readProjection(f.group.Element(), func() bool {
		for _, current := range validations {
			if current == nil || !current() {
				return false
			}
		}
		return true
	})
}

// Settle accepts only the active attempt. Success describes the submitted snapshot;
// Current reports whether it still describes the draft now visible to the user.
func (f *Form) Settle(id SubmissionID, outcome SubmissionOutcome) bool {
	requireMutable(f.Element())
	if f.attempt.ID != id || f.attempt.State != SubmissionPending {
		return false
	}
	f.complete(outcome)
	return true
}

func (f *Form) complete(outcome SubmissionOutcome) {
	state := SubmissionSucceeded
	if outcome.Cancelled {
		state = SubmissionCancelled
	} else if outcome.Err != nil {
		state = SubmissionFailed
	}
	f.attempt.State, f.attempt.Err = state, outcome.Err
	f.problem = nil
	f.cancelFeedback.clear()
	f.Element().Invalidate()
}

// Submission returns the current attempt, with Current computed from the exact submitted field revisions.
func (f *Form) Submission() Submission {
	result := f.attempt
	result.Current = result.State != SubmissionIdle
	if len(f.submitted) > 0 {
		f.group.Element().ReadOnly(func() {
			for _, field := range f.submitted {
				if field.field.Revision() != field.revision {
					result.Current = false
					break
				}
			}
		})
	}
	return result
}

// Cancel requests cancellation of the pending attempt. It does not settle the attempt; the worker or application must deliver Settle.
func (f *Form) Cancel() error {
	requireMutable(f.Element())
	if !f.canCancel() {
		return ErrNotCancellable
	}
	return f.cancel(f.attempt.ID)
}

func (f *Form) canCancel() bool { return f.cancel != nil && f.attempt.State == SubmissionPending }

func (f *Form) submissionAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[struct{}, Submission]{
		ID: ActionReadSubmission, Label: "Read submission",
		Run: func(context.Context, struct{}) (scene.ActionResult[Submission], error) {
			return scene.ActionResult[Submission]{Value: f.Submission()}, nil
		},
	}).Binding()
}

func (f *Form) cancelAction() scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[SubmissionID, Submission]{
		ID: ActionCancel, Label: "Cancel submission",
		Availability: func(id SubmissionID) scene.Availability {
			if !f.canCancel() || id != f.attempt.ID {
				return scene.Availability{Disabled: true, Reason: "submission is not current or cancellable"}
			}
			return scene.Availability{}
		},
		Run: func(_ context.Context, id SubmissionID) (scene.ActionResult[Submission], error) {
			err := f.Cancel()
			state := scene.Unchanged
			if f.attempt.ID != id || f.attempt.State != SubmissionPending {
				state = scene.Applied
			} else if err == nil {
				state = scene.Started
			}
			return scene.ActionResult[Submission]{State: state, Value: f.Submission()}, err
		},
	}).Binding()
}

func (f *Form) checkSubmissionField(field Field, collect func(Field) error) error {
	if collect == nil {
		return field.Validate()
	}
	if err := collect(field); err != nil {
		return err
	}
	return readProjection(f.group.Element(), field.Error)
}

func (f *Form) fieldProblem() error {
	for _, field := range f.fields {
		if err := field.Error(); err != nil {
			return err
		}
	}
	return nil
}

func (f *Form) validateSubmission(id SubmissionID, collect func(Field) error) ([]fieldVersion, error) {
	var validations []func() bool
	var versions []fieldVersion
	var firstInvalid Field
	var problem error
	for _, field := range f.fields {
		err := f.checkSubmissionField(field, collect)
		if f.attempt.ID != id {
			return nil, ErrSubmissionStale
		}
		if err != nil && (collect != nil || errors.Is(err, ErrValidationSuperseded)) {
			return nil, err
		}
		if err != nil && firstInvalid == nil {
			firstInvalid, problem = field, err
		}
		f.group.Element().ReadOnly(func() {
			validations = append(validations, field.Validation())
			versions = append(versions, fieldVersion{field: field, revision: field.Revision()})
		})
	}
	if !f.currentValidations(validations) {
		return nil, ErrValidationSuperseded
	}
	if firstInvalid != nil {
		f.problem = f.fieldProblem
		_ = firstInvalid.Element().RequestFocusFirst()
		return nil, problem
	}
	if err := f.checkDraft(id, validations); err != nil {
		return nil, err
	}
	return versions, nil
}

func (f *Form) checkDraft(id SubmissionID, validations []func() bool) error {
	if f.check == nil {
		return nil
	}
	err := f.check()
	if f.attempt.ID != id {
		return ErrSubmissionStale
	}
	if !f.currentValidations(validations) {
		return ErrValidationSuperseded
	}
	if err != nil {
		f.problem = func() error {
			if f.currentValidations(validations) {
				return err
			}
			return nil
		}
	}
	return err
}
