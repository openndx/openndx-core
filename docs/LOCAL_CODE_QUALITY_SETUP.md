# Local Code Quality Setup Guide

This guide describes how to set up and run comprehensive code quality checks locally for the OpenNDX MVP project.

## Overview

The OpenNDX MVP project uses a comprehensive code quality system that includes:

- **Code Formatting** (gofumpt, goimports)
- **Linting** (go vet, gofmt, staticcheck)
- **Security Scanning** (gosec)
- **Testing** (unit tests with coverage)

All quality checks are orchestrated through a centralized Makefile that automatically detects and processes all Go services in the project.

## Prerequisites

### Required Tools

1. **Go** (version 1.21+)

   ```bash
   go version
   ```

2. **Make** (GNU Make)

   ```bash
   make --version
   ```

3. **Git** (for repository operations)
   ```bash
   git --version
   ```

## Installation & Setup

### 1. Clone the Repository

```bash
git clone https://github.com/OpenNDX/openndx-mvp.git
cd openndx-mvp
```

### 2. Install Go Quality Tools

The Makefile will automatically install required tools when you run quality checks for the first time. However, you can install them manually:

```bash
# Install formatting tools
go install mvdan.cc/gofumpt@latest
go install golang.org/x/tools/cmd/goimports@latest

# Install linting tools
go install honnef.co/go/tools/cmd/staticcheck@latest

# Install security scanner
brew install gosec  # macOS with Homebrew
# OR
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

### 3. VS Code Setup (Optional but Recommended)

The project includes VS Code configuration for real-time linting:

1. Install the Go extension for VS Code
2. The workspace settings in `.vscode/settings.json` will automatically configure:
   - Real-time formatting with gofumpt
   - Import management with goimports
   - Static analysis with staticcheck
   - Automatic formatting on save

## Usage

### Running Quality Checks

#### Check All Services

```bash
make quality-check-all
```

#### Check Specific Service

```bash
make quality-check <service-name>
```

Available services:

- `portal-backend`
- `orchestration-engine`
- `consent-engine`
- `policy-decision-point`

#### Individual Quality Steps

```bash
# Format code
make format <service-name>

# Run linting
make lint <service-name>

# Run security checks
make security <service-name>

# Run tests
make test <service-name>
```

### Quality Check Pipeline

Each quality check runs the following steps:

1. **Code Formatting**

   - `gofumpt`: Stricter formatting than gofmt
   - `goimports`: Organize and format imports

2. **Linting**

   - `go vet`: Built-in Go static analysis
   - `gofmt -d`: Check formatting compliance
   - `staticcheck`: Advanced static analysis

3. **Security Scanning**

   - `gosec`: Security vulnerability scanner
   - Detects common security issues
   - Reports confidence and severity levels

4. **Testing**
   - Unit tests with coverage reporting
   - HTML coverage reports generated
   - Coverage thresholds monitored

## Quality Standards

### Formatting

- All Go code must pass `gofumpt` formatting
- Imports must be organized with `goimports`
- No formatting violations allowed

### Linting

- Must pass `go vet` without errors
- Must pass `staticcheck` (warnings are non-blocking)
- Code must follow Go best practices

### Security

- `gosec` security scans are performed
- Security issues are reported but non-blocking
- Review and address HIGH severity issues

### Testing

- All tests must pass
- Aim for >70% code coverage
- Critical paths should have comprehensive tests

## Output Examples

### Successful Quality Check

```
Quality checking portal-backend...
Running comprehensive quality checks for Go service: portal-backend
✅ Code formatted for Go service portal-backend
✅ Basic lint checks completed for Go service portal-backend
✅ Staticcheck completed for Go service portal-backend
✅ Security check completed for Go service portal-backend
✅ Tests passed for Go service portal-backend
✅ All quality checks passed for Go service portal-backend
```

### Security Issues Found

```
Running security checks for Go service: consent-engine
Results:

[/path/to/file.go:762] - G107 (CWE-88): Potential HTTP request made with variable url
(Confidence: MEDIUM, Severity: MEDIUM)
  > 762:        resp, err := http.Get(userJwksURL)

Summary:
  Gosec  : 2.22.10
  Files  : 10
  Lines  : 2889
  Issues : 3

⚠️  Security issues found in consent-engine (non-blocking)
✅ Security check completed for Go service consent-engine
```

## Troubleshooting

### Common Issues and Solutions

#### 1. Tools Not Installed

**Error:**

```
gofumpt: command not found
```

**Solution:**

```bash
go install mvdan.cc/gofumpt@latest
# Ensure $GOPATH/bin is in your PATH
export PATH=$PATH:$(go env GOPATH)/bin
```

#### 2. Go Module Issues

**Error:**

```
go: cannot find main module
```

**Solution:**

```bash
# Navigate to the specific service directory
cd portal-backend
go mod tidy
```

#### 3. Database Connection Errors in Tests

**Error:**

```
failed to connect to database: password authentication failed
```

**Solution:**
Tests that require databases are automatically skipped if no database is available. This is expected behavior for local development.

#### 4. Staticcheck Cache Issues

**Error:**

```
staticcheck: analysis cache corruption
```

**Solution:**

```bash
# Clear staticcheck cache
staticcheck -clean
```

#### 5. Permission Issues with gosec

**Error:**

```
gosec: permission denied
```

**Solution:**

```bash
# If installed via Homebrew
brew reinstall gosec

# If installed via go install
go clean -modcache
go install github.com/securego/gosec/v2/cmd/gosec@latest
```

#### 6. Make Command Not Found

**Error:**

```
make: command not found
```

**Solution:**

```bash
# macOS
xcode-select --install

# Ubuntu/Debian
sudo apt-get install build-essential

# CentOS/RHEL
sudo yum groupinstall "Development Tools"
```

#### 7. Service Path Not Found

**Error:**

```
Error: Service path not found for: invalid-service
```

**Solution:**
Check available services with:

```bash
ls -d */go.mod | sed 's|/go.mod||' | grep -v '^go.mod$'
```

Valid service names:

- `portal-backend`
- `exchange/orchestration-engine` (use `orchestration-engine`)
- `exchange/consent-engine` (use `consent-engine`)
- `exchange/policy-decision-point` (use `policy-decision-point`)

### Environment Setup Issues

#### PATH Configuration

Ensure Go tools are in your PATH:

```bash
echo $PATH | grep -q "$(go env GOPATH)/bin" || echo "Add $(go env GOPATH)/bin to PATH"
```

Add to your shell profile (`.bashrc`, `.zshrc`):

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

#### Go Environment Verification

```bash
go env GOPATH
go env GOROOT
go env GOPROXY
```

### Performance Tips

1. **Parallel Execution**: Use `make quality-check-all` for checking all services efficiently
2. **Incremental Checks**: Use service-specific checks during development
3. **IDE Integration**: Use VS Code settings for real-time feedback
4. **Cache Management**: Regularly clean Go module cache if experiencing issues

## Integration with CI/CD

The quality checks are designed to integrate seamlessly with CI/CD pipelines:

```yaml
# Example GitHub Actions step
- name: Run Quality Checks
  run: make quality-check-all
```

## Coverage Reports

HTML coverage reports are generated for each service:

- `<service>/coverage.html` - Detailed line-by-line coverage
- View in browser for interactive coverage analysis

## Best Practices

### Development Workflow

1. **Before Committing**: Run `make quality-check <service>` on modified services
2. **Before Pull Request**: Run `make quality-check-all` to ensure all services pass
3. **Address Issues**: Fix formatting and linting issues before security review
4. **Review Security**: Analyze gosec findings for legitimate security concerns

### Code Quality Standards

- Write tests for new functionality
- Maintain or improve code coverage
- Address staticcheck suggestions when reasonable
- Review and mitigate security findings
- Follow Go best practices and idioms

## Support

If you encounter issues not covered in this guide:

1. Check the Makefile for specific service configurations
2. Review individual service README files
3. Ensure all dependencies are properly installed
4. Verify Go module integrity with `go mod tidy`
5. Check VS Code settings in `.vscode/settings.json`

For project-specific questions, refer to the main project documentation or open an issue in the repository.
