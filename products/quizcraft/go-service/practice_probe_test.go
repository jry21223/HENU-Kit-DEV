package quizcraft

import "testing"

func TestPracticeProbeSubmissionUsesOneValidBlankCandidate(t *testing.T) {
	expected := []any{"", "program", "programs"}
	submitted, ok := practiceProbeSubmission("blank", expected)
	if !ok || submitted != "program" {
		t.Fatalf("practiceProbeSubmission(blank) = %#v, %t", submitted, ok)
	}
	if !scoreAnswer("blank", submitted, expected, nil) {
		t.Fatal("representative blank submission did not pass production scoring")
	}
}

func TestPracticeProbeSubmissionRejectsEmptyMultipleChoice(t *testing.T) {
	if submitted, ok := practiceProbeSubmission("multi", []any{}); ok {
		t.Fatalf("empty multiple-choice submission was accepted: %#v", submitted)
	}
}
