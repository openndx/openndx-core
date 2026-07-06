# Makefile Usage Guide

This guide provides a quick reference for all available Makefile commands in the OpenDIF MVP project.

> 📚 **For detailed code quality setup and troubleshooting**, see [LOCAL_CODE_QUALITY_SETUP.md](LOCAL_CODE_QUALITY_SETUP.md)

## Quick Start

```bash
# Check all services
make quality-check-all

# Check specific service
make quality-check portal-backend
```

## Available Services

The following Go services are available for quality checks:

- `portal-backend` - Main Portal Backend
- `orchestration-engine` - Data orchestration engine
- `consent-engine` - Consent management engine
- `policy-decision-point` - Policy decision service

## Command Reference

### 🔍 Quality Check Commands

#### Check All Services

```bash
make quality-check-all
```

Runs comprehensive quality checks on all Go services in the project.

#### Check Specific Service

```bash
make quality-check <service-name>
```

**Examples:**

```bash
make quality-check portal-backend
make quality-check consent-engine
make quality-check policy-decision-point
```

Runs the complete quality pipeline for a specific service:

1. Code formatting (gofumpt, goimports)
2. Linting (go vet, gofmt, staticcheck)
3. Security scanning (gosec)
4. Unit tests with coverage

### 🎨 Individual Quality Steps

#### Format Code

```bash
make format <service-name>
```

**Examples:**

```bash
make format portal-backend
make format orchestration-engine
```

Formats Go code using:

- `gofumpt` - Stricter formatting than gofmt
- `goimports` - Organize and format imports

#### Run Linting

```bash
make lint <service-name>
```

**Examples:**

```bash
make lint orchestration-engine
make lint consent-engine
```

Performs static analysis using:

- `go vet` - Built-in Go static analysis
- `gofmt -d` - Check formatting compliance
- `staticcheck` - Advanced static analysis

#### Security Scanning

```bash
make security <service-name>
```

**Examples:**

```bash
make security portal-backend
make security policy-decision-point
```

Scans for security vulnerabilities using `gosec`.

#### Run Tests

```bash
make test <service-name>
```

**Examples:**

```bash
make test portal-backend
make test consent-engine
```

Executes unit tests with coverage reporting.

### 🛠️ Utility Commands

#### Install Quality Tools

```bash
make install-tools
```

Installs all required Go quality tools:

- gofumpt, goimports (formatting)
- staticcheck (linting)
- gosec (security)

#### Clean Build Artifacts

```bash
make clean
```

Removes build artifacts and coverage reports.

#### Help

```bash
make help
```

Displays available commands and usage information.

## Command Output

### ✅ Successful Quality Check

```
Quality checking portal-backend...
Running comprehensive quality checks for Go service: portal-backend
✅ Code formatted for Go service portal-backend
✅ Basic lint checks completed for Go service portal-backend
✅ Staticcheck completed for Go service portal-backend
✅ Security check completed for Go service portal-backend
✅ Tests passed for Go service portal-backend
Coverage report generated: portal-backend/coverage.html
total: (statements) 57.4%
✅ All quality checks passed for Go service portal-backend
```

### ⚠️ Quality Issues Found

```
Running security checks for Go service: consent-engine
Results:

[/path/to/file.go:762] - G107 (CWE-88): Potential HTTP request made with variable url
(Confidence: MEDIUM, Severity: MEDIUM)

Summary:
  Gosec  : 2.22.10
  Files  : 10
  Lines  : 2889
  Issues : 3

⚠️  Security issues found in consent-engine (non-blocking)
✅ Security check completed for Go service consent-engine
```

### ❌ Quality Check Failure

```
❌ Tests failed for Go service portal-backend
Error: Test suite returned non-zero exit code
```

## Development Workflow

### 🔄 Recommended Usage Pattern

1. **During Development:**

   ```bash
   # Format and check your changes
   make format portal-backend
   make lint portal-backend
   ```

2. **Before Committing:**

   ```bash
   # Full quality check for modified service
   make quality-check portal-backend
   ```

3. **Before Pull Request:**
   ```bash
   # Check all services
   make quality-check-all
   ```

### 🎯 Targeted Fixes

```bash
# Fix formatting issues
make format <service>

# Address linting problems
make lint <service>

# Review security findings
make security <service>

# Verify tests pass
make test <service>
```

## Performance Tips

- **Use specific service names** for faster iteration during development
- **Run `quality-check-all`** before major commits to catch cross-service issues
- **Parallel execution** is automatic for `quality-check-all`
- **Coverage reports** are generated as HTML files for detailed analysis

## Integration Examples

### Git Hooks

```bash
# Pre-commit hook
#!/bin/sh
make quality-check-all
```

### CI/CD Pipeline

```yaml
- name: Quality Checks
  run: make quality-check-all
```

### VS Code Tasks

```json
{
  "label": "Quality Check Current Service",
  "type": "shell",
  "command": "make quality-check ${workspaceFolderBasename}"
}
```

## Error Handling

The Makefile includes robust error handling:

- **Path Resolution:** Maps service names to correct directory paths
- **Tool Validation:** Checks for required tools and provides installation guidance
- **Graceful Failures:** Non-blocking warnings for security and linting issues
- **Clear Messaging:** Color-coded output with emoji indicators

## Advanced Usage

### Custom Service Paths

If you have services in non-standard locations, you must manually update the service path variables in the Makefile to include them.

### Selective Quality Checks

```bash
# Only format and lint (skip security and tests)
make format portal-backend && make lint portal-backend
```

### Coverage Analysis

Coverage reports are automatically generated:

```
<service>/coverage.html  # Detailed line-by-line coverage
```

## Troubleshooting Quick Reference

| Issue             | Quick Fix                                 |
| ----------------- | ----------------------------------------- |
| Tool not found    | `make install-tools`                      |
| Service not found | Check service name against available list |
| Permission denied | `chmod +x` on Makefile or reinstall tools |
| Go module issues  | `cd <service> && go mod tidy`             |
| Cache corruption  | `go clean -modcache`                      |

## Need Help?

- **Setup Issues:** See [LOCAL_CODE_QUALITY_SETUP.md](LOCAL_CODE_QUALITY_SETUP.md) for detailed installation and troubleshooting
- **Command Help:** Run `make help` for quick command reference
- **Service Issues:** Check individual service README files
- **Quality Standards:** Review quality requirements in the setup guide

---

📚 **For comprehensive setup instructions, troubleshooting, and quality standards, see [LOCAL_CODE_QUALITY_SETUP.md](LOCAL_CODE_QUALITY_SETUP.md)**
