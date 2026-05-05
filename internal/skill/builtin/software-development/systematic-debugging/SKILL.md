---
name: systematic-debugging
description: "4-phase root cause debugging: understand bugs before fixing."
---

# Systematic Debugging

## Overview

Random fixes waste time and create new bugs. Quick patches mask underlying issues.

**Core principle:** ALWAYS find root cause before attempting fixes. Symptom fixes are failure.

## The Iron Law

```
NO FIXES WITHOUT ROOT CAUSE INVESTIGATION FIRST
```

## When to Use

Use for ANY technical issue:
- Test failures
- Bugs in production
- Unexpected behavior
- Performance problems
- Build failures
- Integration issues

**Use this ESPECIALLY when:**
- Under time pressure
- "Just one quick fix" seems obvious
- You've already tried multiple fixes
- Previous fix didn't work
- You don't fully understand the issue

## The Four Phases

You MUST complete each phase before proceeding to the next.

---

## Phase 1: Root Cause Investigation

### 1. Read Error Messages Carefully

- Don't skip past errors or warnings
- Read stack traces completely
- Note line numbers, file paths, error codes

**Action:** Use `read_file` on the relevant source files. Use `grep` to find the error string in the codebase.

### 2. Reproduce Consistently

- Can you trigger it reliably?
- What are the exact steps?

**Action:** Use `bash` to run the failing test or trigger the bug.

### 3. Check Recent Changes

- What changed that could cause this?
- Git diff, recent commits

**Action:**

```bash
git log --oneline -10
git diff
git log -p --follow src/problematic_file.py | head -100
```

### 4. Gather Evidence in Multi-Component Systems

For EACH component boundary:
- Log what data enters and exits the component
- Verify environment/config propagation
- Check state at each layer

### 5. Trace Data Flow

- Where does the bad value originate?
- Keep tracing upstream until you find the source
- Fix at the source, not at the symptom

**Action:** Use `grep` to trace references.

### Phase 1 Completion Checklist

- [ ] Error messages fully read and understood
- [ ] Issue reproduced consistently
- [ ] Recent changes identified and reviewed
- [ ] Evidence gathered
- [ ] Root cause hypothesis formed

**STOP:** Do not proceed to Phase 2 until you understand WHY it's happening.

---

## Phase 2: Pattern Analysis

### 1. Find Working Examples
- Locate similar working code in the same codebase

### 2. Compare Against References
- Read the reference implementation COMPLETELY

### 3. Identify Differences
- List every difference between working and broken

### 4. Understand Dependencies
- What settings, config, environment assumptions?

---

## Phase 3: Hypothesis and Testing

### 1. Form a Single Hypothesis
- State clearly: "I think X is the root cause because Y"

### 2. Test Minimally
- Make the SMALLEST possible change
- One variable at a time

### 3. Verify Before Continuing
- Did it work? -> Phase 4
- Didn't work? -> Form NEW hypothesis
- DON'T add more fixes on top

---

## Phase 4: Implementation

### 1. Create Failing Test Case
- Simplest possible reproduction
- MUST have before fixing

### 2. Implement Single Fix
- Address the root cause identified
- ONE change at a time
- No "while I'm here" improvements

### 3. Verify Fix

Run the specific test and the full suite.

### 4. The Rule of Three

- If < 3 failed fixes: Return to Phase 1
- If >= 3 failed fixes: STOP and question the architecture

---

## Red Flags

If you catch yourself thinking:
- "Quick fix for now, investigate later"
- "Just try changing X and see if it works"
- "Add multiple changes, run tests"
- "One more fix attempt" (when already tried 2+)

**STOP. Return to Phase 1.**
