# Fundament Overview

## Functional Goals

- Developer-friendly platform for deploying and operating applications.
- Multi-tenant by design, enabling organizational growth and flexible tenant management.
- Managed infrastructure that abstracts compute, storage, and networking for ease of use and consistency.
- Managed orchestration layer to simplify workload scheduling, scaling, and lifecycle management.
- Built-in platform services for common needs such as application delivery, compute execution, and data persistence.
- Extensible through pluggable services, allowing tenants to autonomously evolve and customize their technology stack.
- Horizontally scalable foundation capable of supporting thousands of tenants and large-scale infrastructure footprints.
- Strict tenant isolation with a shared-nothing architecture for compute and storage, ensuring reliability and predictable performance.

And fundamentally:

- Secure by default.
- Highly available under heavy demand or failure conditions.
- Fast, delivering responsive operations at every layer.

## Non-goals

- Public Cloud, we are not in the business of creating a public cloud offering. Fundament is specifically designed to be self-hosted by companies and large organizations, giving them an Autonomous Private Cloud.

## Open Source Mindset

Fundament builds on top of existing Open Source projects. This has a number of benefits:

- Building on the expertise and community support of existing projects.
- Enabling us to customize and extend existing project to meet our specific needs.
- Giving back to the community by adding features and improvements to existing projects.
- No need to re-invent the wheel; more time and energy can go to developing and improving the Fundament platform.

Fundament strongly avoids proprietary software and closed-source solutions.
