from score_parentheses import score_parentheses


def test_score_parentheses_basic() -> None:
    assert score_parentheses("()") == 1


def test_score_parentheses_composition() -> None:
    assert score_parentheses("()()") == 2


def test_score_parentheses_nesting() -> None:
    assert score_parentheses("(())") == 2


def test_score_parentheses_mixed() -> None:
    assert score_parentheses("(()(()))") == 6
