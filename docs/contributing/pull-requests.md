# Pull Request Guidelines

We welcome pull requests from the community! This guide will help ensure your contributions are reviewed and merged smoothly.

## Before You Start

1.   **Check for existing work:** Search [open pull requests](https://github.com/OpenDIF/opendif-mvp/pulls) to avoid duplicate work
2.   **Discuss major changes:** For significant changes, consider opening an issue first to discuss the approach
3.   **Read the documentation:** Review the [Development Guide](development.md) to set up your environment

## Pull Request Workflow

1.  **Fork the repository** (if you haven't already)
2.  **Create a branch** from `main`:
    ```bash
    git checkout main
    git pull upstream main
    git checkout -b feature/your-feature-name
    # or
    git checkout -b fix/issue-123
    ```

3.  **Make your changes** following our [Development Guide](development.md)

4.  **Test your changes:**
    - Run all tests: `go test ./...`
    - Run integration tests if applicable
    - Ensure builds succeed: `make validate-build-all`

5.  **Commit your changes:**
    - Write clear, descriptive commit messages
    - Keep commits focused and logical
    - Reference issues: `Fixes #123` or `Closes #456`

6.  **Push to your fork:**
    ```bash
    git push origin feature/your-feature-name
    ```

7.  **Open a pull request** to the `main` branch

## Pull Request Guidelines

### Size and Scope

-   **Keep it focused:** Each PR should address a single issue or feature
-   **Keep it small:** Smaller PRs are easier to review and merge faster
-   **Break it up:** If your change is large, consider splitting it into multiple PRs

### Code Quality

-   **Follow style guidelines:** Run `go fmt ./...` before committing
-   **Add tests:** Include tests for new functionality
-   **Update tests:** Update existing tests if behavior changes
-   **All tests pass:** Ensure all tests pass locally before submitting

### Documentation

-   **Update docs:** If your changes affect user-facing features, update relevant documentation
-   **Add comments:** Comment complex logic or non-obvious decisions
-   **Update README:** Update README if setup or usage changes

### Pull Request Description

Use the pull request template and include:

-   **Clear summary:** What does this PR do and why?
-   **Type of change:** Bug fix, new feature, refactoring, etc.
-   **Changes made:** Detailed list of what was modified
-   **Testing:** How was this tested?
-   **Related issues:** Link to related issues (e.g., `Fixes #123`)

### Review Process

-   **Be patient:** Reviews may take time, especially for large changes
-   **Respond to feedback:** Address review comments promptly and constructively
-   **Update as needed:** Make requested changes and push updates to the same branch
-   **Mark as ready:** Re-request review when you've addressed feedback

### After Approval

-   **Don't force push:** Avoid force pushing after review starts (unless requested)
-   **Keep branch updated:** Rebase or merge `main` into your branch if needed
-   **Squash commits:** Maintainers may squash commits when merging

## Checklist

Before submitting, ensure:

-   [ ] Code follows project style guidelines
-   [ ] All tests pass locally
-   [ ] New code includes appropriate tests
-   [ ] Documentation is updated if needed
-   [ ] Commit messages are clear and descriptive
-   [ ] PR description is complete and uses the template
-   [ ] Related issues are linked
-   [ ] No merge conflicts with `main` branch

## Getting Help

-   Review [Development Guide](development.md) for setup instructions
-   Check [Reporting Issues](reporting-issues.md) for bug reporting
-   See [Code of Conduct](../../CODE_OF_CONDUCT.md) for community standards

[View Open Pull Requests](https://github.com/OpenDIF/opendif-mvp/pulls)
