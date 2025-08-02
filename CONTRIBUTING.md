# Contributing to Kubernetes MCP Operator

First off, thanks for taking the time to contribute! We welcome all kinds of contributions: bug reports, feature suggestions, documentation improvements, and pull requests.

The Kubernetes MCP (Model Context Protocol) Operator simplifies the integration of AI agents with existing web services by automatically generating protocol layers from OpenAPI specifications.

## How to Contribute

### 🐛 Reporting Bugs

If you've found a bug:

- Ensure it hasn't already been reported in Issues.
- Include clear steps to reproduce the issue.
- Provide the following information:
  - Kubernetes version (`kubectl version`)
  - Operator version
  - MCPServer custom resource YAML
  - Controller logs (`kubectl logs -n <namespace> deployment/mcp-operator-controller-manager`)
  - Any error messages from pods or jobs
- Add screenshots or logs if possible.
- Use the **Bug Report** issue template.

### 🌟 Suggesting Features

Want to improve something? Great!

- Check if a similar suggestion already exists.
- Explain the problem your feature would solve.
- Be clear about your proposed solution.
- Consider how it fits with our architecture (see [ADR-001](docs/adr/ADR-001.md)).
- Use the **Feature Request** issue template.

### 💻 Submitting Code

#### Prerequisites

Before you start developing, ensure you have:

- Go version v1.24.0+
- Docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster
- [Kind](https://kind.sigs.k8s.io/) (for local testing)
- [Kubebuilder](https://book.kubebuilder.io/) (installed via dev container or manually)

#### Development Setup

1. **Fork** the repository.
2. **Clone** your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/kmcp.git
   cd kmcp
   ```

3. **Set up your development environment** (optional but recommended):
   ```bash
   # Use the provided dev container if you prefer
   # The dev container includes all required tools
   ```

4. **Install dependencies**:
   ```bash
   make help  # See all available commands
   go mod tidy
   ```

#### Development Workflow

1. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Run tests before making changes**:
   ```bash
   make test          # Run unit tests
   make test-e2e      # Run end-to-end tests (requires Kind cluster)
   make lint          # Run linter
   ```

3. **Make your changes**:
   - Follow Go best practices and conventions
   - Update documentation if needed
   - Add tests for new functionality
   - Ensure your code follows the existing patterns

4. **Verify your changes**:
   ```bash
   make fmt           # Format Go code
   make vet           # Run go vet
   make test          # Run tests
   make lint          # Run linter
   make manifests     # Generate manifests if you changed APIs
   make generate      # Generate code if needed
   ```

5. **Test your changes locally**:
   ```bash
   # Build and test the operator locally
   make docker-build IMG=test-operator:latest
   make docker-kind-load IMG=test-operator:latest
   make deploy IMG=test-operator:latest
   
   # Test with a sample MCPServer
   kubectl apply -f config/samples/mcp_v1_mcpserver.yaml
   
   # Check logs
   kubectl logs -n kmcp-system deployment/kmcp-controller-manager -f
   ```

6. **Clean up**:
   ```bash
   make undeploy      # Remove operator from cluster
   make cleanup-test-e2e  # Clean up test cluster
   ```

#### Code Style and Standards

- Follow standard Go formatting (`make fmt`)
- Use meaningful variable and function names
- Add comments for exported functions and complex logic
- Write tests for new functionality
- Follow Kubernetes API conventions for CRDs
- Use structured logging with the controller-runtime logger

#### Testing

We use multiple testing approaches:

- **Unit Tests**: Test individual functions and methods
  ```bash
  make test
  ```

- **End-to-End Tests**: Test the complete operator workflow
  ```bash
  make test-e2e
  ```

- **Linting**: Ensure code quality
  ```bash
  make lint
  ```

#### Project Structure

```
├── api/v1/                     # API definitions (CRDs)
├── cmd/                        # Main application entry point
├── config/                     # Kubernetes manifests and Kustomize configs
│   ├── crd/                   # Custom Resource Definitions
│   ├── default/               # Default deployment configuration
│   ├── manager/               # Controller manager configuration
│   ├── rbac/                  # RBAC configurations
│   └── samples/               # Example MCPServer resources
├── docs/                       # Documentation
│   └── adr/                   # Architecture Decision Records
├── internal/controller/        # Controller implementation
│   └── templates/             # Container build templates
├── test/                       # Test files
│   ├── e2e/                   # End-to-end tests
│   └── utils/                 # Test utilities
├── hack/                       # Development scripts
└── Makefile                    # Build and development commands
```

#### Working with MCPServer CRD

When modifying the MCPServer API:

1. Edit `api/v1/mcpserver_types.go`
2. Run `make manifests generate` to update generated code
3. Update the controller logic in `internal/controller/mcpserver_controller.go`
4. Add or update tests
5. Update documentation and examples

#### Useful Makefile Targets

```bash
make help              # Show all available targets
make build             # Build the operator binary
make run              # Run operator locally (development mode)
make docker-build     # Build Docker image
make install          # Install CRDs into cluster
make deploy           # Deploy operator to cluster
make undeploy         # Remove operator from cluster
make test             # Run unit tests
make test-e2e         # Run end-to-end tests
make lint             # Run linter
make fmt              # Format code
make manifests        # Generate Kubernetes manifests
make generate         # Generate code
make helm             # Generate Helm chart
```

### 📝 Documentation

Documentation improvements are always welcome:

- Fix typos or unclear explanations
- Add examples and use cases
- Improve API documentation
- Update architecture decision records (ADRs)

### 🔄 Pull Request Process

1. **Ensure your PR**:
   - Has a clear title and description
   - Includes tests for new functionality
   - Updates documentation if needed
   - Passes all CI checks (tests, linting)
   - Follows the existing code style

2. **Link related issues**:
   - Reference any related issues in your PR description
   - Use keywords like "Fixes #123" to auto-close issues

3. **Be patient**:
   - Wait for maintainer review
   - Be responsive to feedback
   - Update your PR based on review comments

### 🚀 Release Process

Releases are automated through GitHub Actions:

- Pushes to `main` trigger development releases
- Tagged releases create stable versions
- Helm charts are automatically published to GHCR

### 🤝 Community Guidelines

- Be respectful and constructive in discussions
- Follow the [Kubernetes Code of Conduct](https://kubernetes.io/community/code-of-conduct/)
- Help newcomers and answer questions
- Share your use cases and success stories

### 📚 Additional Resources

- [Architecture Decision Record ADR-001](docs/adr/ADR-001.md)
- [High Level Design](HLD.md)
- [Kubebuilder Documentation](https://book.kubebuilder.io/)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Model Context Protocol Specification](https://modelcontextprotocol.io/)

### ❓ Getting Help

If you need help:

- Check existing [Issues](https://github.com/v2dY/kmcp/issues)
- Review the [documentation](README.md)
- Look at [example MCPServer resources](config/samples/)
- Join our community discussions

Thank you for contributing to the Kubernetes MCP Operator! 🎉
