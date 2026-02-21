---
title: Infrastructure
---

Unlike platforms such as OpenStack or CloudStack -which aim to deliver every conceivable service “out of the box” and end up being heavyweight and complex- Fundament takes the opposite approach. It is intentionally minimal at the lower layers of the stack, offering only the essentials by default. This simplicity keeps the foundation clean and reliable. From there, functionality grows on a per-cluster basis: each organization extends their environment with exactly the Tools they need, nothing more.

![Infrastructure Stack](assets/infrastructure-stack.drawio.svg)

### Why Gardener

[Gardener](https://gardener.cloud/) is a battle-tested, production-grade Kubernetes management solution originally developed by SAP to run thousands of clusters at scale. It has proven reliability in real-world 𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒 scenarios where uptime, consistency, and security are essential.

Gardener’s architecture cleanly separates the control plane from the shoot clusters, ensuring that workloads remain isolated while still benefiting from central governance. This design makes it straightforward to support multi-tenancy, self-service cluster provisioning, and automated lifecycle management.

Another major benefit is underlying infrastructure neutrality. Gardener supports all major public cloud providers, as well as private data centers via bare metal and virtualization layers. This infrastructure neutrality means Fundament could potentially support alternative infrastructure providers beyond metal-stack in the future, preventing vendor lock-in.

Gardener's extensibility and strong community backing mean Fundament is not locked into a rigid system. With well-defined extension points, operators can add custom logic for networking, storage, identity, and other layers. At the same time, Fundament benefits from an active open-source community and ongoing 𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒 contributions, ensuring the project evolves with industry best practices.

Gardener has a large community in Germany and is funded by the European Union / NextGenerationEU. It is a project of NeoNephos Foundation, which is part of Linux Foundation Europe.

### Why metal-stack

[metal-stack](https://metal-stack.io/) provides the bare-metal infrastructure that Fundament relies on to stay simple, performant, and transparent at its core. Unlike traditional virtualization layers, metal-stack provisions physical servers directly in an automated and Kubernetes-native way. This means organizations get the raw performance and predictable behavior of true bare metal, without the overhead of a hypervisor.

By embracing bare metal, Fundament can guarantee consistent network throughput, storage performance, and latency, which are often compromised in virtualized environments. For a platform that aims to provide flexible tooling to its users, having a stable and predictable infrastructure layer is critical.

metal-stack also aligns perfectly with Fundament’s philosophy: minimal by default, extensible where needed. It doesn’t drown operators in layers of abstraction, but instead delivers just enough automation to make managing bare-metal infrastructure practical at scale.

metal-stack is designed to support a broad range of hardware vendors and configurations, ensuring there’s no lock-in to a single supplier. Its focus on open standards and automated provisioning means operators can manage diverse server fleets (Dell, HPE, Supermicro, etc., or custom builds) with the same workflows. This flexibility not only protects against vendor dependency but also allows Fundament to run on the hardware that best fits performance, cost, or availability requirements.

The metal-stack project is relatively young and makes use of modern technologies. It has been in production use at financial institutions since 2020, with active development by a dedicated team and a growing community.
