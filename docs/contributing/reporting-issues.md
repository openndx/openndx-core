# Reporting Issues

We use GitHub Issues to track bugs, feature requests, and improvements. Your detailed reports help us improve the project!

## Before Creating an Issue

1.   **Search existing issues:** Check [open issues](https://github.com/OpenDIF/opendif-mvp/issues) and [closed issues](https://github.com/OpenDIF/opendif-mvp/issues?q=is%3Aissue+is%3Aclosed) to see if your issue has already been reported
2.   **Check documentation:** Review the README and relevant documentation to see if your question is answered
3.   **Verify it's a bug:** For behavior questions, ensure it's actually a bug and not expected behavior

## Reporting a Bug

If you've found a bug, please create a new issue using the bug report template and include:

### Required Information

1.  **Clear and descriptive title**
    - Use a concise summary (e.g., "Consent Engine fails to validate expired consents")
    - Avoid vague titles like "Bug" or "Problem"

2.  **Steps to reproduce**
    - Provide step-by-step instructions
    - Include code snippets or commands if applicable
    - Be specific about inputs and actions

3.  **Expected behavior**
    - Describe what should happen

4.  **Actual behavior**
    - Describe what actually happens
    - Include error messages or logs if available

5.  **Environment details**
    - OS and version (e.g., macOS 14.0, Ubuntu 22.04)
    - Go version: `go version`
    - Docker version (if applicable): `docker --version`
    - Service versions or commit hashes

### Additional Information

-   **Screenshots:** If applicable, include screenshots or screen recordings
-   **Error logs:** Include relevant log output or stack traces
-   **Minimal reproduction:** If possible, provide a minimal code example that reproduces the issue
-   **Related issues:** Link to related issues or pull requests

### Example Bug Report

```markdown
**Title:** Consent Engine returns 500 error when querying expired consents

**Steps to reproduce:**
1. Create a consent with expiration date in the past
2. Query GET /consents?status=expired
3. Observe 500 Internal Server Error

**Expected behavior:**
Should return list of expired consents with 200 OK

**Actual behavior:**
Returns 500 Internal Server Error with message "database connection failed"

**Environment:**
- OS: macOS 14.0
- Go: 1.21.5
- Service: consent-engine v1.2.0
```

## Requesting a Feature

Have an idea for a new feature or improvement? We'd love to hear it! Use the feature request template and include:

### Required Information

1.  **Clear and descriptive title**
    - Summarize the feature (e.g., "Add bulk consent approval API endpoint")

2.  **Detailed description**
    - Explain what the feature should do
    - Describe the user experience or API design

3.  **Problem it solves**
    - What problem does this feature address?
    - What use case does it enable?

4.  **Proposed solution**
    - How should this feature work?
    - Include API design, UI mockups, or examples if applicable

### Additional Information

-   **Alternatives considered:** What other approaches did you consider?
-   **Impact:** Who would benefit from this feature?
-   **Implementation notes:** Any technical considerations or constraints?

### Example Feature Request

```markdown
**Title:** Add bulk consent approval API endpoint

**Description:**
Currently, consent approvals must be done one at a time via PATCH /consents/:id.
For data owners managing many consents, this is inefficient.

**Proposed Solution:**
Add POST /consents/bulk-approve endpoint that accepts an array of consent IDs
and approves them in a single transaction.

**Use Case:**
Data owners with 100+ pending consents can approve them all at once instead of
making 100 individual API calls.
```

## Issue Labels

We use labels to categorize issues:

-   `bug` - Something isn't working
-   `enhancement` - New feature or improvement
-   `documentation` - Documentation improvements
-   `good first issue` - Good for newcomers
-   `help wanted` - Extra attention needed
-   `question` - Further information is requested

## After Submitting

-   **Be responsive:** Respond to questions or requests for clarification
-   **Provide updates:** If you find more information, add it to the issue
-   **Close if resolved:** If you resolved the issue yourself, let us know and close it
-   **Be patient:** We'll review your issue as soon as possible

## Security Issues

**Do not** report security vulnerabilities through public GitHub issues. Instead, please contact the maintainers directly or use GitHub's [private vulnerability reporting](https://github.com/OpenDIF/opendif-mvp/security/advisories/new) feature.

[Open a new issue](https://github.com/OpenDIF/opendif-mvp/issues/new)
