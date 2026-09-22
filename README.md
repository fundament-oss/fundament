# Fundament

Fundament is the Dutch sovereign government cloud: an open-source cloud platform, and the live proof-of-concept environment on which it runs. The NDS Cloud programme, an all-of-government programme that aims to realise a Government Cloud with the requirement to be as sovereign as currently feasible (see the [project brief sent to the Dutch Parliament](https://open.overheid.nl/details/b20dc73e-f67d-4cfa-aeaf-a3556b94d63e), in Dutch), builds on Fundament, and its design is aligned with Fundament's architecture.

As open-source software, Fundament can also be run by any organization as its own Autonomous Private Cloud, separate from the sovereign government cloud. It is designed to provide organizations with a minimal, reliable foundation for running modern applications. Unlike traditional platforms that bundle every service by default, Fundament is basic at its core -delivering only the essential infrastructure and orchestration layers- while allowing each tenant to extend their environment with exactly the services and tools they need.

On top of this foundation, Fundament serves as an internal developer platform (IDP): enabling developers to build, deploy, and operate applications with speed and confidence. Its API-first design, multi-tenant architecture, and focus on autonomy and scalability make it suitable for organizations that want cloud capabilities on their own terms; secure, reliable, and without unnecessary complexity.

## Project status

**Preparing phase 1 production**

The current PoC environment is being used for tests with users. A [Fieldlab](https://digilab.overheid.nl/) with a multi-user, multi-day test is planned for 23–25 November 2026. Scaling up towards a first production environment with a number of launching customers is in preparation.

A broader architecture document, Het Ontwerp, that describes the context, goals and starting points of the NDS Cloud design is currently in [public review](https://www.digitaleoverheid.nl/nieuws-nds/nds-cloud-mijlpaal-publicatie-van-het-ontwerp/) (in Dutch). The NDS Cloud design has its own architecture decision records, separate from the [Architecture Decision Records](docs/adr/README.adoc) in this repository but largely compatible with them.

## Documentation

- [Overview](docs/user/overview.md)
- [Infrastructure](docs/user/infrastructure.md)
- [Organizations and projects](docs/user/organizations.md)
- [Plugins](docs/user/plugins.md)
- [Fundament development](docs/developer/fundament/getting-started.md)
- [Plugin development](docs/developer/plugins/index.md)

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md).

## License

The contents of this repository are copyrighted by *The Fundament Authors*
(`git log --format='%aN <%aE>' | sort -u`).

- **Source code** is licensed under the [GNU Affero General Public License (AGPL)](https://www.gnu.org/licenses/agpl-3.0.html), unless otherwise stated.
- **Documentation** (including Markdown, D2 and similar files) is licensed under [CC BY-SA](https://creativecommons.org/licenses/by-sa/4.0/).
