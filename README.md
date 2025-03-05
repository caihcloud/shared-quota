# SharedQuota
**A cross-namespace resource quota system for Kubernetes.**

## Description

This project provides a custom resource definition (CRD) and controller for implementing resource quotas that can span across multiple Kubernetes namespaces.  Unlike the built-in `ResourceQuota` which is scoped to a single namespace, `SharedQuota` allows you to define a quota that applies to a *group* of namespaces.  This enables centralized management of resource consumption across different teams or environments, fostering more efficient resource allocation and preventing individual namespaces from consuming excessive resources within the cluster.

The underlying logic of `SharedQuota` is similar to the standard `ResourceQuota` in that it tracks resource usage (CPU, memory, pods, etc.) across the specified namespaces. Once the total usage reaches the defined limit, the controller restricts the creation or modification of resources in those namespaces that would exceed the quota. This ensures predictable resource availability and prevents resource starvation within the Kubernetes cluster. It acts as a global or overarching ResourceQuota system.

```yaml
apiVersion: quota.caih.com/v1
kind: SharedQuota
metadata:
  name: sharedquota-sample
spec:
  selector:
    environment: production # label selector for namespaces
  quota:
    hard:
      pods: "10"
      cpu: "20"
      memory: "40Gi"
      requests.storage: "100Gi"
      requests.cpu: "10"
      requests.memory: "30Gi"
      requetes.nvidia.com/gpu: "2"
      limits.cpu: "20"
      limits.memory: "40Gi"
      persistentvolumeclaims: "10"
```

This is particularly useful for organizations:

*   Wanting to provide a single, unified resource allocation across multiple teams.
*   Needing to manage resource consumption across different development, staging, and production environments residing in separate namespaces within the same cluster.
*   Require more flexible and dynamic allocation strategies than standard ResourceQuotas can provide.
*   Centralizing quota management and visibility for administrators.

## Features

*   **Cross-Namespace Quotas:** Enforce resource limits that apply to groups of namespaces rather than single namespaces.
*   **Familiar Semantics:** Maintains a similar conceptual model to standard `ResourceQuota`, leveraging familiar Kubernetes resource types and concepts.
*   **Centralized Management:** Define and manage quotas from a central point, simplifying administration and promoting consistency.
*   **Resource Tracking:** Accurately tracks resource usage across the participating namespaces.
*   **Admission Control:** Prevents the creation or modification of resources that would violate the shared quota limits.
*   **Status Reporting:** Provides real-time insight into resource usage in each namespace, how close they are to reaching the limit.

## Getting Started

### Prerequisites
- go version v1.23.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
0.  **Ensure that `cert-manager` is installed in your Kubernetes cluster:**
**NOTE:** If you don’t have `cert-manager` installed, you can install it using the following command:
    ```bash
    kubectl apply -f deploy/cert-manager/cert-manager.yaml
    ```
1.  **Prepare the tls cert:**
    ```bash
    kubectl apply -f deploy/1.cert.yaml
2.  **Install the CRD:**
    ```bash
    kubectl apply -f deploy/2.customresourcedefinition.yaml
    ```
3.  **Deploy the Controlle:**
    ```bash
    kubectl apply -f deploy/3.deployment.yaml
    ```
4.  **Add Webhook Config:**
**NOTE:** Ensure the controller deployment is running properly; otherwise, the webhook might not be reachable, which could prevent Pod creation.
    ```bash
    kubectl apply -f deploy/4.webhook.yaml
    ```
### All-In-One
**Delete the instances (CRs) from the cluster:**

```sh
kubectl apply -f deploy/all-in-one.yaml
```

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -f deploy/all-in-one.yaml
```

## Contributing

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

