---
description: Deep code review of a file. Usage: /review <path>
---

Please do a thorough code review of `$1`.

Read the file first, then evaluate it on these dimensions:

## 1. Correctness
- Logic errors, off-by-one conditions, incorrect assumptions
- Edge cases that aren't handled (nil, empty, overflow, etc.)
- Race conditions or concurrency bugs

## 2. Error Handling
- Are all errors checked?
- Are errors wrapped with context?
- Do error messages give the caller enough information?

## 3. API Design
- Is the interface intuitive for callers?
- Are parameter names and types clear?
- Is there anything surprising or non-idiomatic?

## 4. Performance
- Any obvious inefficiencies (unnecessary allocations, O(n²) loops, etc.)?
- Is anything missing that would matter under load?

## 5. Test Coverage
- What code paths lack test coverage?
- Are there any hard-to-test designs that should be refactored?

## 6. Security (if applicable)
- Input validation
- Path traversal, injection, or privilege-escalation risks

---

Format your response as a numbered list of findings, most critical first.
For each finding:
- Quote the relevant code
- Explain the problem clearly
- Suggest a specific fix

End with an overall assessment (Approved / Approved with minor changes / Needs work).
