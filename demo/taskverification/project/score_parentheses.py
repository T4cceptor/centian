def score_parentheses(text: str) -> int:
    stack = [0]

    for ch in text:
        if ch == "(":
            stack.append(0)
        else:
            inner = stack.pop()
            stack[-1] += 1 if inner == 0 else 2 * inner

    return stack[0]
