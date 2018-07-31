## Introduction

This helm chart creates an apisprout deployment in a kubernetes cluster. Unfortunately it is not part of a helm repository yet, so you will have to download and install with the chart locally.


## Installation

```
helm install --name {release-name} --set apiyamlpath={open-api-yaml-path} {chart directory}
```

## Uninstalling

```
helm delete --purge {release-name}
```

## Values

| Parameter                  | Description               | Default              |
|:---------------------------|:--------------------------|:---------------------|
| `apiyamlpath`              | Path to OpenAPI Yaml file | Required: no default |
| `appname`                  | The name of the app       | apisprout            |
| `replicas`                 | Number of replicas        | 1                    |
| `deployment.containerPort` | The container port        | 8000                 |
| `service.annotations`      | Service annotations       | None                 |
| `service.type`             | The type of service       | LoadBalancer         |
| `service.port`             | The service port          | 8000                 |


