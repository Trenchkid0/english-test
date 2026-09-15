package main

import "testing"

func TestReviewInterval(t *testing.T) {
	tests := []struct {
		mastery int
		correct bool
		want    int
	}{{0, false, 1}, {1, true, 3}, {3, true, 7}, {5, true, 14}}
	for _, test := range tests {
		if got := reviewInterval(test.mastery, test.correct); got != test.want {
			t.Fatalf("reviewInterval(%d,%t)=%d, want %d", test.mastery, test.correct, got, test.want)
		}
	}
}

func TestAssignReviewKeysIsStable(t *testing.T) {
	questions := []question{{Type: "reading", Context: "A short passage.", Prompt: "What is stated?", Choices: []string{"A", "B", "C", "D"}}}
	assignReviewKeys(questions)
	first := questions[0].ReviewKey
	questions[0].ReviewKey = ""
	assignReviewKeys(questions)
	if first == "" || questions[0].ReviewKey != first || len(first) != 64 {
		t.Fatalf("unstable review key: %q then %q", first, questions[0].ReviewKey)
	}
}

func TestWritingHelpers(t *testing.T) {
	if got := countWords(" One\n two   three "); got != 3 {
		t.Fatalf("countWords=%d, want 3", got)
	}
	feedback := demoWritingFeedback("First paragraph.\n\nSecond paragraph.\n\nThird paragraph.", 180)
	if feedback.Overall < 0 || feedback.Overall > 9 || feedback.Summary == "" || len(feedback.NextSteps) == 0 {
		t.Fatalf("invalid demo feedback: %#v", feedback)
	}
	if _, ok := writingTaskPrompt("task2"); !ok {
		t.Fatal("task2 prompt missing")
	}
	if _, ok := writingTaskPrompt("unknown"); ok {
		t.Fatal("unknown writing task accepted")
	}
}
