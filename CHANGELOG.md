# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog], and this project adheres to
[Semantic Versioning].

<!-- references -->

[keep a changelog]: https://keepachangelog.com/en/1.0.0/
[semantic versioning]: https://semver.org/spec/v2.0.0.html

## Unreleased

### Added

- Add `KubernetesEnvironmentTargetDiscoverer`, which discoveres Kubernetes
  services in the same namespace as the application

## [0.1.1] - 2021-01-20

### Added

- Add `DefaultGRPCPort` constant

### Changed

- `DNSTargetDiscoverer` now uses `DefaultGRPCPort` by default

## [0.1.0] - 2021-01-20

- Initial release

<!-- references -->

[unreleased]: https://github.com/dogmatiq/discoverkit
[0.1.0]: https://github.com/dogmatiq/discoverkit/releases/tag/v0.1.0
[0.1.1]: https://github.com/dogmatiq/discoverkit/releases/tag/v0.1.1

<!-- version template
## [0.0.1] - YYYY-MM-DD

### Added
### Changed
### Deprecated
### Removed
### Fixed
### Security
-->
