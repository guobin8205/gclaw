---
name: test-driven-development
description: "TDD: enforce RED-GREEN-REFACTOR, tests before code."
---

# Test-Driven Development (TDD)

## The Iron Law

```
NO PRODUCTION CODE WITHOUT A FAILING TEST FIRST
```

Write code before the test? Delete it. Start over.

## When to Use

**Always:**
- New features
- Bug fixes
- Refactoring
- Behavior changes

**Exceptions (ask the user first):**
- Throwaway prototypes
- Generated code
- Configuration files

## Red-Green-Refactor Cycle

### RED - Write Failing Test

Write one minimal test showing what should happen.

Requirements:
- One behavior per test
- Clear descriptive name ("and" in name? Split it)
- Real code, not mocks (unless truly unavoidable)

### Verify RED - Watch It Fail

**MANDATORY. Never skip.**

Confirm:
- Test fails (not errors from typos)
- Failure message is expected
- Fails because the feature is missing

Test passes immediately? You're testing existing behavior. Fix the test.

### GREEN - Minimal Code

Write the simplest code to pass the test. Nothing more.

**Cheating is OK in GREEN:**
- Hardcode return values
- Copy-paste
- Duplicate code

We'll fix it in REFACTOR.

### Verify GREEN - Watch It Pass

**MANDATORY.** Run the specific test, then ALL tests to check for regressions.

### REFACTOR - Clean Up

After green only:
- Remove duplication
- Improve names
- Extract helpers
- Simplify expressions

Keep tests green throughout.

### Repeat

Next failing test for next behavior. One cycle at a time.

## Red Flags - STOP and Start Over

- Code before test
- Test passes immediately on first run
- Can't explain why test failed
- Tests added "later"
- Rationalizing "just this once"
- "Already spent X hours, deleting is wasteful"

**All of these mean: Delete code. Start over with TDD.**

## Verification Checklist

Before marking work complete:

- [ ] Every new function/method has a test
- [ ] Watched each test fail before implementing
- [ ] Wrote minimal code to pass each test
- [ ] All tests pass
- [ ] Tests use real code (mocks only if unavoidable)
- [ ] Edge cases and errors covered
