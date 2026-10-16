package cmd

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nimbu/cli/internal/bootstrap"
)

func bundleFixture() []bootstrap.Bundle {
	return []bootstrap.Bundle{{ID: "blog-articles", Label: "Blog + articles"}}
}

func initApplyModel(t *testing.T) *initTeaModel {
	t.Helper()
	model := newInitTeaTestModel(initPromptModel{})
	model.transcript = []initTranscriptEntry{{Label: "Template", Value: "Starterskit"}}
	model.phase = initTeaPhaseApply
	model.loadingSummary = "Creating project"
	model.loadingDetail = "Writing files..."
	return model
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	return exitErr.Code
}

// SIGTERM reaches the program as a QuitMsg. Before the fix the wizard
// returned nil and printed "Done!" with exit 0.
func TestInitTeaSigtermDuringPromptIsCancellation(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{Sites: []initSiteChoice{{Label: "Demo"}}})

	if got := initTeaFilter(model, tea.QuitMsg{}); got != (tea.QuitMsg{}) {
		t.Fatalf("expected quit to pass through outside apply, got %#v", got)
	}
	err := initTeaOutcome(model, nil)
	if code := exitCode(t, err); code != initExitTerminated {
		t.Fatalf("expected exit %d, got %d", initExitTerminated, code)
	}
	if !errors.Is(err, errInitCancelled) {
		t.Fatalf("expected init cancelled, got %v", err)
	}
}

func TestInitTeaQuitBeforeDoneIsNeverSuccess(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.phase = initTeaPhasePrompt

	if err := initTeaOutcome(model, nil); !errors.Is(err, errInitCancelled) {
		t.Fatalf("expected a program that stopped before done to be cancelled, got %v", err)
	}
}

func TestInitTeaInterruptedRunExits130(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	err := initTeaOutcome(model, tea.ErrInterrupted)
	if code := exitCode(t, err); code != initExitInterrupted {
		t.Fatalf("expected exit %d, got %d", initExitInterrupted, code)
	}
}

func TestInitTeaDoneIsSuccess(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	updated, cmd := model.Update(initTeaBootstrapDoneMsg{result: initResult{Path: "/tmp/theme-demo"}})
	model = updated.(*initTeaModel)
	if cmd == nil {
		t.Fatal("expected done to quit the program")
	}
	// The wizard's own quit must pass the filter untouched.
	if got := initTeaFilter(model, tea.QuitMsg{}); got != (tea.QuitMsg{}) {
		t.Fatalf("expected own quit to pass, got %#v", got)
	}
	if err := initTeaOutcome(model, nil); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestInitTeaCtrlCInPromptExits130(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{Sites: []initSiteChoice{{Label: "Demo"}}})
	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	model = updated.(*initTeaModel)
	if cmd == nil || !model.quitting {
		t.Fatal("expected ctrl+c to quit")
	}
	if !strings.Contains(initView(model), "Cancelled.") {
		t.Fatalf("expected closing line in final frame, got:\n%s", initView(model))
	}
	if code := exitCode(t, initTeaOutcome(model, nil)); code != initExitInterrupted {
		t.Fatalf("expected exit %d, got %d", initExitInterrupted, code)
	}
}

func TestInitTeaEscClearsFilterBeforeCancelling(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{Sites: []initSiteChoice{{Label: "Zorgpoort"}, {Label: "Zenjoy"}}})
	model.filterInput.SetValue("zen")
	if !strings.Contains(initView(model), "esc clear filter") {
		t.Fatalf("expected hint to say esc clears the filter, got:\n%s", initView(model))
	}

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(*initTeaModel)
	if cmd != nil || model.quitting || model.err != nil {
		t.Fatalf("expected first esc to only clear the filter (cmd=%v quitting=%v err=%v)", cmd != nil, model.quitting, model.err)
	}
	if got := model.filterInput.Value(); got != "" {
		t.Fatalf("expected filter cleared, got %q", got)
	}
	if !strings.Contains(initView(model), "esc cancel") {
		t.Fatalf("expected hint to mention esc cancel, got:\n%s", initView(model))
	}

	updated, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(*initTeaModel)
	if cmd == nil || !errors.Is(model.err, errInitCancelled) {
		t.Fatalf("expected second esc to cancel, err=%v", model.err)
	}
	err := initTeaOutcome(model, nil)
	var exitErr *ExitError
	if !errors.Is(err, errInitCancelled) || errors.As(err, &exitErr) {
		t.Fatalf("expected plain init cancelled (exit 1), got %v", err)
	}
}

func TestInitTeaCancelKeysAreHeldWhileWritingFiles(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: 'c', Mod: tea.ModCtrl}} {
		model := initApplyModel(t)
		updated, cmd := model.Update(key)
		model = updated.(*initTeaModel)
		if cmd != nil || model.quitting {
			t.Fatalf("%s: expected no quit while writing files", key.String())
		}
		if model.phase != initTeaPhaseApply || !model.cancelHeld {
			t.Fatalf("%s: expected apply phase to continue with a held cancel", key.String())
		}
		if view := initView(model); !strings.Contains(view, "Finishing") {
			t.Fatalf("%s: expected finishing notice, got:\n%s", key.String(), view)
		}
	}
}

// A signal during apply is held until the files are written, then reported.
func TestInitTeaSignalDuringApplyWaitsForBootstrap(t *testing.T) {
	model := initApplyModel(t)

	held := initTeaFilter(model, tea.InterruptMsg{})
	if _, ok := held.(initTeaHoldMsg); !ok {
		t.Fatalf("expected interrupt to be held during apply, got %#v", held)
	}
	updated, cmd := model.Update(held)
	model = updated.(*initTeaModel)
	if cmd != nil || !model.cancelHeld {
		t.Fatal("expected held interrupt to keep the program running")
	}

	updated, cmd = model.Update(initTeaBootstrapDoneMsg{result: initResult{Path: "/tmp/theme-demo"}})
	model = updated.(*initTeaModel)
	if cmd == nil {
		t.Fatal("expected bootstrap completion to quit")
	}
	err := initTeaOutcome(model, nil)
	if code := exitCode(t, err); code != initExitInterrupted {
		t.Fatalf("expected exit %d, got %d", initExitInterrupted, code)
	}
	if !strings.Contains(err.Error(), "/tmp/theme-demo") {
		t.Fatalf("expected error to say where the project was written, got %v", err)
	}
}

func TestInitTeaSigtermDuringApplyIsHeld(t *testing.T) {
	model := initApplyModel(t)
	if _, ok := initTeaFilter(model, tea.QuitMsg{}).(initTeaHoldMsg); !ok {
		t.Fatal("expected SIGTERM quit to be held during apply")
	}
	if model.signal != initSignalTerminate {
		t.Fatalf("expected terminate recorded, got %v", model.signal)
	}
}

func TestInitWorkShutdownWaitsForJobsBeforeCleanup(t *testing.T) {
	work := newInitWork(context.Background())
	if !work.begin() {
		t.Fatal("expected begin before shutdown")
	}
	var jobDone, cleanedAfterJob atomic.Bool
	work.onShutdown(func() { cleanedAfterJob.Store(jobDone.Load()) })

	release := make(chan struct{})
	go func() {
		<-work.ctx.Done() // shutdown cancels the job's context first
		<-release
		jobDone.Store(true)
		work.end()
	}()

	finished := make(chan struct{})
	go func() {
		work.shutdown()
		close(finished)
	}()
	select {
	case <-finished:
		t.Fatal("shutdown returned while a job was still running")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-finished

	if !cleanedAfterJob.Load() {
		t.Fatal("expected cleanup to run after the job finished")
	}
	if work.begin() {
		t.Fatal("expected begin to fail after shutdown")
	}
}

func TestInitTeaSpaceTogglesMultiSelect(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{BundleOptions: bundleFixture()})
	model.enterStep(initTeaStepBundles)

	model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if !model.isSelected("blog-articles") {
		t.Fatal("expected space to select the bundle under the cursor")
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if model.isSelected("blog-articles") {
		t.Fatal("expected a second space to deselect it")
	}
}

func TestInitTeaLoadingShowsEscHint(t *testing.T) {
	model := newInitTeaTestModel(initPromptModel{})
	model.phase = initTeaPhaseLoading
	model.loadingSummary, model.loadingDetail = "Template", "Cloning repository..."
	if view := initView(model); !strings.Contains(view, "esc cancel") {
		t.Fatalf("expected loading view to say how to cancel, got:\n%s", view)
	}
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil || !errors.Is(updated.(*initTeaModel).err, errInitCancelled) {
		t.Fatal("expected esc to cancel while loading")
	}
}
