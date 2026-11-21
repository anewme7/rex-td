package rex

import "testing"

func TestCompletedCodexTurnReadsOnlyTheLastFinishedTurn(t *testing.T) {
	previous := "› Check setup\n\n• Setup is ready.\n\n  Worked for 12s • 19:21\n\n"
	latest := "› Verify the new shortcut\n\n• I will try it.\n\n• Ran checks\n\n• The shortcut works.\n\n  Worked for 27s • 19:23\n"
	composer := "\n› Ask Codex to do anything\n"
	if got := completedCodexTurn(previous + latest + composer); got != (PaneTurn{"Verify the new shortcut", "The shortcut works."}) {
		t.Fatalf("expected the last completed input and result, got %+v", got)
	}
	if got := completedCodexTurn(previous + latest + "\n› Next question\n\n• I will check.\n" + composer); got != (PaneTurn{}) {
		t.Fatalf("a newer active turn must not show the old result: %+v", got)
	}
	if got := completedCodexTurn(previous + "› Run the check\n\n• I will try it.\n\n• Ran checks\n\n  Worked for 4s • 19:24\n" + composer); got != (PaneTurn{Input: "Run the check"}) {
		t.Fatalf("tool activity must not be presented as a result: %+v", got)
	}
}
