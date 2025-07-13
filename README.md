# Kubernetes MCP generator operator
The MCP Generator Operator simplifies the development of agentic applications by automating the creation of a protocol layer between AI agents and existing systems that expose HTTP APIs. By parsing OpenAPI specifications and generating a Model Context Protocol (MCP), it enables seamless, structured interaction between autonomous agents and external services, accelerating integration and reducing manual overhead.

## Description

The MCP (Model Context Protocol) Generator Operator is designed to streamline the integration of AI agents with existing web services in Kubernetes environments. It automates the generation of protocol layers that act as an interface between autonomous agents and services defined by HTTP APIs, such as RESTful backends and microservices.

At its core, the operator watches for custom Kubernetes resources that specify an OpenAPI specification, Upon detecting a new specification, it automatically triggers code generation workflows—using tools like OpenAPI Generator—to produce a context-aware agent interface layer. This layer standardizes how agents interact with APIs, ensuring consistent, structured communication.

This approach is especially useful in agentic systems where an LLM-driven agent must query, invoke, or reason about external APIs. Instead of manually writing boilerplate wrappers or SDKs, developers can declaratively define API sources and let the operator handle the rest.

### Use Cases

* Accelerating development of AI-native applications by reducing API integration overhead.

 * Enabling autonomous systems to reliably and safely interact with microservices or legacy systems via OpenAPI-based interfaces.

 * Maintaining consistent communication patterns across multiple services or environments without manual intervention.

 * By embedding the generation and deployment of these interface layers directly into the Kubernetes workflow, the MCP Generator Operator bridges the gap between large language models and existing infrastructure.

## Demo
[Video](media/demo.mp4)
[Code](docs/demo.yml)
## Install
```bash
helm install release-name oci://ghcr.io/v2dy/project/chart/helm:0.1.0
```

```bash
cat <<EOF | kubectl apply -f -
apiVersion: mcp.my.domain/v1
kind: MCPServer
metadata:
  namespace: default
  labels:
    app.kubernetes.io/name: operator
    app.kubernetes.io/managed-by: kustomize
  name: mcpserver-sample4
spec:
  replicas: 1
  url: "https://raw.githubusercontent.com/open-meteo/open-meteo/refs/heads/main/openapi.yml"
  basePath: "https://api.open-meteo.com"
EOF

```
example [CRD](config/samples//mcp_v1_mcpserver.yaml)


## Demo

## Docs
 [ADR-001](docs/adr/ADR-001.md)
 [HLD](docs/HLD.md)

# Development



## Getting Started

### Prerequisites
- go version v1.24.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

