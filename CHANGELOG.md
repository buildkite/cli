# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

## [v1.1.0](https://github.com/buildkite/cli/tree/v1.1.0) (2020-05-08)
[Full Changelog](https://github.com/buildkite/cli/compare/v1.0.0...v1.1.0)

### Changed
- Fix local pipeline running for Windows [#73](https://github.com/buildkite/cli/pull/73) (@crufter)
- Add --listen-port to allow a stable port to be chosen [#70](https://github.com/buildkite/cli/pull/70) [#71](https://github.com/buildkite/cli/pull/71) (@petemounce)
- Update github.com/99designs/keyring to v1.1.3 [#69](https://github.com/buildkite/cli/pull/69) (@lox)


## [v1.0.0](https://github.com/buildkite/cli/tree/v1.0.0) (2019-06-21)
[Full Changelog](https://github.com/buildkite/cli/compare/v0.5.0...v1.0.0)

### Changed
- Support listing meta-data keys [#62](https://github.com/buildkite/cli/pull/62) (@lox)
- Add --env flag to `bk build create` [#61](https://github.com/buildkite/cli/pull/61) (@slam)
- Add support for passing metadata to bk local run. [#56](https://github.com/buildkite/cli/pull/56) (@stefan-improbable)
- Fix spelling of GitHub for configure command [#53](https://github.com/buildkite/cli/pull/53) (@JuanitoFatas)

## [v0.5.0](https://github.com/buildkite/cli/tree/v0.5.0) (2019-04-18)
[Full Changelog](https://github.com/buildkite/cli/compare/v0.4.1...v0.5.0)

### Added
- Fix bug where file backend is default over keychain [#51](https://github.com/buildkite/cli/pull/51) (@lox)
- Handle wildcards in branch patterns [#49](https://github.com/buildkite/cli/pull/49) (@lox)

### Fixed
- Fix spelling in error messages [#50](https://github.com/buildkite/cli/pull/50) (@jsleeio)

## [v0.4.1](https://github.com/buildkite/cli/tree/v0.4.1) (2019-03-13)
[Full Changelog](https://github.com/buildkite/cli/compare/v0.4.0...v0.4.1)

### Fixed
- Fix (and test) pipeline level env [#45](https://github.com/buildkite/cli/pull/45) (@lox)
- Propagate step environment to job environment [#44](https://github.com/buildkite/cli/pull/44) (@lox)
- Fix default keychain selection [#38](https://github.com/buildkite/cli/pull/38) (@lachlancooper)

## [v0.4.0](https://github.com/buildkite/cli/tree/v0.4.0) (2019-02-17)
[Full Changelog](https://github.com/buildkite/cli/compare/v0.3.0...v0.4.0)

### Changed
- Implement text and select block steps [#25](https://github.com/buildkite/cli/pull/25) (@lox)
- Add MIT license [#34](https://github.com/buildkite/cli/pull/34) (@lox)
- Add plugins-path and switch to temp dirs [#30](https://github.com/buildkite/cli/pull/30) (@toolmantim)
- Add some more tests and fixes for pipeline parsing [#24](https://github.com/buildkite/cli/pull/24) (@lox)

## [v0.3.0](https://github.com/buildkite/cli/tree/v0.3.0) (2019-02-04)
[Full Changelog](https://github.com/buildkite/cli/compare/v0.2.0...v0.3.0)

### Changed
- Expose more of the keyring options [#22](https://github.com/buildkite/cli/pull/22) (@lox)
- Add a run alias for local run [#21](https://github.com/buildkite/cli/pull/21) (@lox)
- Provide an empty builds dir and close the bootstrap script [#20](https://github.com/buildkite/cli/pull/20) (@lox)
- Rename commands [#13](https://github.com/buildkite/cli/pull/13) (@lox)
- Reduce polling intervals in local pipeline processor [#10](https://github.com/buildkite/cli/pull/10) (@lox)

## [v0.2.0](https://github.com/buildkite/cli/tree/v0.2.0) (2018-10-31)
[Full Changelog](https://github.com/buildkite/cli/compare/v0.1.0...v0.2.0)

### Changed
- Add a Homebrew tap release process [#7](https://github.com/buildkite/cli/pull/7) (@toolmantim)
- Show the correct path for config file. [#9](https://github.com/buildkite/cli/pull/9) (@lox)
- Readme cleanups [#8](https://github.com/buildkite/cli/pull/8) (@toolmantim)

## [v0.1.0](https://github.com/buildkite/cli/tree/v0.1.0) (2018-10-18)
[Full Changelog](https://github.com/buildkite/cli/compare/2a544ab29355...v0.1.0)

### Changed
- Local run command [#6](https://github.com/buildkite/cli/pull/6) (@lox)
- Update to golang 1.11 and modules [#5](https://github.com/buildkite/cli/pull/5) (@lox)
