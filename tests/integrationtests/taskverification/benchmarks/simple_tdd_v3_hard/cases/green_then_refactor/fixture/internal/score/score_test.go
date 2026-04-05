package score

import "testing"

func TestScoreParentheses(t *testing.T) {
	if score := ScoreParentheses("(())"); score != 2 {
		t.Fatalf("expected score 2, got %d", score)
	}
}
