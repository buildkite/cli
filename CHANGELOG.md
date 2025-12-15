 
# Changelog

## Unreleased
- Fix pipeline picker to respect --no-input flag for automation scenarios
- SUP-1693 Bind "s" to stop agent by [[PR #188](https://github.com/buildkite/cli/pull/188)] @jradtilbrook
- SUP-1698: Keybind for viewing agent data within `bk agent list` by [[PR #192](https://github.com/buildkite/cli/pull/192)] @james2791
- Fix runtime error introduced in SUP-1695 by [[PR #194](https://github.com/buildkite/cli/pull/194)] @lizrabuya
- SUP-1695: Keybinding for viewing agent data in web from `bk agent list` by [[PR #193](https://github.com/buildkite/cli/pull/193)] @lizrabuya
- SUP-1658: `bk agent list`: appender, pagination and limiting by [[PR #191](https://github.com/buildkite/cli/pull/191)] @james2791
- SUP-1700: Agent view output metadata improvements by [[PR #187](https://github.com/buildkite/cli/pull/187)] @james2791
- SUP-1699 Change styling of `bk agent list` by [[PR #186](https://github.com/buildkite/cli/pull/186)] @jradtilbrook
- SUP-1682 Parallelise bulk agent stopping by [[PR #184](https://github.com/buildkite/cli/pull/184)] @jradtilbrook
- SUP-1691: `bk agent list` interactivity by [[PR #185](https://github.com/buildkite/cli/pull/185)] @james2791
- SUP-1560 Add bk build new by [[PR #183](https://github.com/buildkite/cli/pull/183)] @lizrabuya
- SUP-1652: Filterable `bk agent list` via REST API query parameters by [[PR #181](https://github.com/buildkite/cli/pull/181)] @james2791
- Resolve project pipeline by [[PR #182](https://github.com/buildkite/cli/pull/182)] @mcncl
- Bump github.com/spf13/viper from 1.18.1 to 1.18.2 by [[PR #179](https://github.com/buildkite/cli/pull/179)] @dependabot
- SUP-1619: Agent bulk stopping by [[PR #178](https://github.com/buildkite/cli/pull/178)] @james2791
- SUP-1567 Add bk agent list by [[PR #175](https://github.com/buildkite/cli/pull/175)] @jradtilbrook
- Bump github.com/charmbracelet/bubbles from 0.16.1 to 0.17.1 by [[PR #177](https://github.com/buildkite/cli/pull/177)] @dependabot
- Bump github.com/charmbracelet/bubbletea from 0.24.2 to 0.25.0 by [[PR #176](https://github.com/buildkite/cli/pull/176)] @dependabot
- SUP-1615 Add flag to open agent in browser by [[PR #174](https://github.com/buildkite/cli/pull/174)] @jradtilbrook
- Bump github.com/spf13/viper from 1.17.0 to 1.18.1 by [[PR #173](https://github.com/buildkite/cli/pull/173)] @dependabot
- SUP-1566 Add agent view command by [[PR #172](https://github.com/buildkite/cli/pull/172)] @jradtilbrook
- Bump github.com/buildkite/go-buildkite/v3 from 3.8.0 to 3.10.0 by [[PR #167](https://github.com/buildkite/cli/pull/167)] @dependabot
- SUP-1566 Augment agent stop with loading spinner by [[PR #171](https://github.com/buildkite/cli/pull/171)] @jradtilbrook
- SUP-1441 Support clustered agent URLs by [[PR #170](https://github.com/buildkite/cli/pull/170)] @jradtilbrook
- Add more build targets to goreleaser by [[PR #169](https://github.com/buildkite/cli/pull/169)] @jradtilbrook
- Bump github.com/spf13/cobra from 1.7.0 to 1.8.0 by [[PR #165](https://github.com/buildkite/cli/pull/165)] @dependabot
- Bump github.com/buildkite/go-buildkite/v3 from 3.6.0 to 3.8.0 by [[PR #164](https://github.com/buildkite/cli/pull/164)] @dependabot
- Bump github.com/spf13/viper from 1.16.0 to 1.17.0 by [[PR #161](https://github.com/buildkite/cli/pull/161)] @dependabot
- Bump golang.org/x/net from 0.10.0 to 0.17.0 by [[PR #163](https://github.com/buildkite/cli/pull/163)] @dependabot
- SUP-1420: README addition by [[PR #159](https://github.com/buildkite/cli/pull/159)] @james2791
- SUP-1450: Added additional Codeowners by [[PR #158](https://github.com/buildkite/cli/pull/158)] @james2791
- Bump github.com/buildkite/go-buildkite/v3 from 3.5.0 to 3.6.0 by [[PR #157](https://github.com/buildkite/cli/pull/157)] @dependabot
- Bump github.com/buildkite/go-buildkite/v3 from 3.4.0 to 3.5.0 by [[PR #154](https://github.com/buildkite/cli/pull/154)] @dependabot
- SUP-1406 Allow configuring multiple organizations by [[PR #153](https://github.com/buildkite/cli/pull/153)] @jradtilbrook
- SUP-1335 Stop agent command by [[PR #152](https://github.com/buildkite/cli/pull/152)] @jradtilbrook
- SUP-1403 Add shared HTTP client by [[PR #151](https://github.com/buildkite/cli/pull/151)] @jradtilbrook
- Init command with checks on existing file. by [[PR #150](https://github.com/buildkite/cli/pull/150)] @mcncl
- Use a configure command to set auth by [[PR #149](https://github.com/buildkite/cli/pull/149)] @mcncl
 
## [v2.0.0](https://github.com/buildkite/cli/compare/v1.2.0...v2.0.0) (2022-07-26)

- Go module renamed with /v2 suffix [#132](https://github.com/buildkite/cli/pull/132) ([pda](https://github.com/pda))
- Remove keyring entirely and use config json [#115](https://github.com/buildkite/cli/pull/115) ([lox](https://github.com/lox))
- golang.org/x/sys updated, was broken with Go 1.18 on macOS [#131](https://github.com/buildkite/cli/pull/131) ([pda](https://github.com/pda))
- Prefer /usr/bin/env bash [#124](https://github.com/buildkite/cli/pull/124) ([wpcarro](https://github.com/wpcarro))
- local: replace "master" with "main" [#129](https://github.com/buildkite/cli/pull/129) ([kevinburke](https://github.com/kevinburke))
- Make BUILDKITE_PIPELINE_DEFAULT_BRANCH configurable [#125](https://github.com/buildkite/cli/pull/125) ([wpcarro](https://github.com/wpcarro))
- Update the URLs for emoji data [FDN-548] [#130](https://github.com/buildkite/cli/pull/130) ([yob](https://github.com/yob))
- Remove end year in LICENSE [#128](https://github.com/buildkite/cli/pull/128) ([JuanitoFatas](https://github.com/JuanitoFatas))
- Amend homebrew install command [#117](https://github.com/buildkite/cli/pull/117) ([l-suzuki](https://github.com/l-suzuki))

## [v1.2.0](https://github.com/buildkite/cli/compare/v1.1.0...v1.2.0) (2021-03-11)

- Pass through Windows env vars in local run [#92](https://github.com/buildkite/cli/pull/92) ([Helcaraxan](https://github.com/Helcaraxan))
- Build linux/arm64 and darwin/arm64 binaries [#107](https://github.com/buildkite/cli/pull/107) ([sj26](https://github.com/sj26))
- Allow meta-data to be passed to the `build create` command [#108](https://github.com/buildkite/cli/pull/108) ([keithpitt](https://github.com/keithpitt))
- Update keyring & go-keychain for macOS 11 fix [#101](https://github.com/buildkite/cli/pull/101) ([pda](https://github.com/pda))
- Convert GitHub auth process from "web application flow" to "device flow" [#100](https://github.com/buildkite/cli/pull/100) ([yob](https://github.com/yob))
- Fix usage example in README [#93](https://github.com/buildkite/cli/pull/93) ([rohansingh](https://github.com/rohansingh))
- CI: compile for macOS and Windows, not just Linux [#90](https://github.com/buildkite/cli/pull/90) ([pda](https://github.com/pda))
- Create linter step to check code quality [#86](https://github.com/buildkite/cli/pull/86) ([Helcaraxan](https://github.com/Helcaraxan))
- Go 1.14; previously a mix 1.11, 1.12 and 1.13 [#88](https://github.com/buildkite/cli/pull/88) ([pda](https://github.com/pda))
- CI pipeline [#89](https://github.com/buildkite/cli/pull/89) ([pda](https://github.com/pda))
- Update the url and version for tap formula [#85](https://github.com/buildkite/cli/pull/85) ([JuanitoFatas](https://github.com/JuanitoFatas))

## [v1.1.0](https://github.com/buildkite/cli/compare/v1.0.0...v1.1.0) (2020-05-15)

- Windows fixes for the cli [#73](https://github.com/buildkite/cli/pull/73) ([crufter](https://github.com/crufter))
- Missing wiring [#71](https://github.com/buildkite/cli/pull/71) ([petemounce](https://github.com/petemounce))
- Add --listen-port to allow a stable port to be chosen [#70](https://github.com/buildkite/cli/pull/70) ([petemounce](https://github.com/petemounce))
- Update github.com/99designs/keyring to v1.1.3 [#69](https://github.com/buildkite/cli/pull/69) ([lox](https://github.com/lox))

## [v1.0.0](https://github.com/buildkite/cli/compare/v0.5.0...v1.0.0) (2019-07-05)

- Support listing meta-data keys [#62](https://github.com/buildkite/cli/pull/62) ([lox](https://github.com/lox))
- Add --env flag to `bk build create` [#61](https://github.com/buildkite/cli/pull/61) ([slam](https://github.com/slam))
- Add support for passing metadata to bk local run. [#56](https://github.com/buildkite/cli/pull/56) ([stefan-improbable](https://github.com/stefan-improbable))
- Fix spelling of GitHub for configure command [#53](https://github.com/buildkite/cli/pull/53) ([JuanitoFatas](https://github.com/JuanitoFatas))

## [v0.5.0](https://github.com/buildkite/cli/compare/v0.4.1...v0.5.0) (2019-04-20)

- Fix bug where file backend is preferred [#51](https://github.com/buildkite/cli/pull/51) ([lox](https://github.com/lox))
- Handle fancy wildcard branch patterns [#49](https://github.com/buildkite/cli/pull/49) ([lox](https://github.com/lox))
- fix spelling in error messages [#50](https://github.com/buildkite/cli/pull/50) ([jsleeio](https://github.com/jsleeio))

## [v0.4.1](https://github.com/buildkite/cli/compare/v0.4.0...v0.4.1) (2019-03-13)

- Fix (and test) pipeline level env [#45](https://github.com/buildkite/cli/pull/45) ([lox](https://github.com/lox))
- Propagate step environment to job environment [#44](https://github.com/buildkite/cli/pull/44) ([lox](https://github.com/lox))
- Fix default keychain selection [#38](https://github.com/buildkite/cli/pull/38) ([lachlancooper](https://github.com/lachlancooper))

## [v0.4.0](https://github.com/buildkite/cli/compare/v0.3.0...v0.4.0) (2019-02-18)

- Implement text and select block steps [#25](https://github.com/buildkite/cli/pull/25) ([lox](https://github.com/lox))
- Add MIT license [#34](https://github.com/buildkite/cli/pull/34) ([lox](https://github.com/lox))
- Add plugins-path and switch to temp dirs [#30](https://github.com/buildkite/cli/pull/30) ([toolmantim](https://github.com/toolmantim))
- Add some more tests and fixes for pipeline parsing [#24](https://github.com/buildkite/cli/pull/24) ([lox](https://github.com/lox))
- Update README.md [#23](https://github.com/buildkite/cli/pull/23) ([aaronsky](https://github.com/aaronsky))

## [v0.3.0](https://github.com/buildkite/cli/compare/v0.2.0...v0.3.0) (2019-02-05)

- Expose more of the keyring options [#22](https://github.com/buildkite/cli/pull/22) ([lox](https://github.com/lox))
- Add a run alias for local run [#21](https://github.com/buildkite/cli/pull/21) ([lox](https://github.com/lox))
- Provide an empty builds dir and close the bootstrap script [#20](https://github.com/buildkite/cli/pull/20) ([lox](https://github.com/lox))
- Rename commands [#13](https://github.com/buildkite/cli/pull/13) ([lox](https://github.com/lox))
- Reduce polling intervals in local pipeline processor [#10](https://github.com/buildkite/cli/pull/10) ([lox](https://github.com/lox))

## [v0.2.0](https://github.com/buildkite/cli/compare/v0.1.0...v0.2.0) (2018-11-01)

- Add a Homebrew tap release process [#7](https://github.com/buildkite/cli/pull/7) ([toolmantim](https://github.com/toolmantim))
- Show the correct path for config file. [#9](https://github.com/buildkite/cli/pull/9) ([lox](https://github.com/lox))
- Readme cleanups [#8](https://github.com/buildkite/cli/pull/8) ([toolmantim](https://github.com/toolmantim))

## [v0.1.0](https://github.com/buildkite/cli/compare/73083884b289...v0.1.0) (2018-10-18)

- Local run command [#6](https://github.com/buildkite/cli/pull/6) ([lox](https://github.com/lox))
- Update to golang 1.11 and modules [#5](https://github.com/buildkite/cli/pull/5) ([lox](https://github.com/lox)) 